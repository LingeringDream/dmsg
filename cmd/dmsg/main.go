package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dmsg/internal/config"
	"dmsg/internal/crypto"
	"dmsg/internal/filter"
	"dmsg/internal/msg"
	dmsgnet "dmsg/internal/net"
	"dmsg/internal/plugin"
	"dmsg/internal/ranking"
	"dmsg/internal/store"
	"dmsg/internal/view"
)

var (
	dataDir    string
	listenAddr string
	bootstrap  []string
	difficulty int
	rendezvous string
	rulesPath  string
)

type session struct {
	cfg     config.Config
	id      *crypto.Identity
	db      *store.Store
	node    *dmsgnet.Node
	fe      *filter.Engine
	views   *view.Manager
	plugins *plugin.Manager
	curView string
}

func main() {
	root := &cobra.Command{
		Use:   "dmsg",
		Short: "Decentralized P2P messaging network",
	}

	root.PersistentFlags().StringVar(&dataDir, "data", defaultDataDir(), "data directory")
	root.PersistentFlags().StringVar(&listenAddr, "listen", "", "listen multiaddr (overrides config)")
	root.PersistentFlags().StringSliceVar(&bootstrap, "bootstrap", nil, "bootstrap peer multiaddrs")
	root.PersistentFlags().IntVar(&difficulty, "difficulty", 0, "PoW difficulty (0 = use config)")
	root.PersistentFlags().StringVar(&rendezvous, "rendezvous", "", "DHT namespace (overrides config)")
	root.PersistentFlags().StringVar(&rulesPath, "rules", "", "filter rules JSON path")

	root.AddCommand(startCmd())
	root.AddCommand(idCmd())
	root.AddCommand(sendCmd())
	root.AddCommand(peersCmd())
	root.AddCommand(followCmd())
	root.AddCommand(muteCmd())
	root.AddCommand(trustCmd())
	root.AddCommand(rulesCmd())
	root.AddCommand(historyCmd())
	root.AddCommand(configCmd())
	root.AddCommand(viewCmd())
	root.AddCommand(pluginCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dmsg")
}

func getRulesPath() string {
	if rulesPath != "" {
		return rulesPath
	}
	return filepath.Join(dataDir, "rules.json")
}

func loadSession() (*session, error) {
	cfg, err := config.Load(dataDir)
	if err != nil {
		return nil, err
	}
	if listenAddr != "" {
		cfg.ListenAddr = listenAddr
	}
	if difficulty > 0 {
		cfg.Difficulty = difficulty
	}
	if rendezvous != "" {
		cfg.Rendezvous = rendezvous
	}
	if len(bootstrap) > 0 {
		cfg.Bootstrap = bootstrap
	}

	id, err := crypto.LoadOrGenerate(dataDir)
	if err != nil {
		return nil, err
	}

	db, err := store.Open(filepath.Join(dataDir, "messages.db"))
	if err != nil {
		return nil, err
	}

	rp := getRulesPath()
	filter.SaveDefaultRules(rp)
	fe := filter.NewEngine()
	fe.LoadRules(rp)

	vm, err := view.NewManager(dataDir)
	if err != nil {
		return nil, err
	}

	pm, err := plugin.NewManager(dataDir)
	if err != nil {
		return nil, err
	}

	return &session{
		cfg:     cfg,
		id:      id,
		db:      db,
		fe:      fe,
		views:   vm,
		plugins: pm,
		curView: cfg.DefaultView,
	}, nil
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the dmsg node (interactive shell)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}

			fmt.Printf("👤 User ID: %s\n", s.id.UserID[:16]+"...")

			handler := func(m *msg.Message) {
				if s.fe.Check(m) == filter.ActionBlock {
					return
				}
				s.db.Save(m)
				ts := time.Unix(m.Timestamp, 0).Format("15:04:05")
				fmt.Printf("\r📨 [%s] %s: %s\n> ", ts, m.PubKey[:8], m.Content)
			}

			node, err := dmsgnet.NewNode(dmsgnet.Config{
				ListenAddr:    s.cfg.ListenAddr,
				Bootstrap:     s.cfg.Bootstrap,
				Rendezvous:    s.cfg.Rendezvous,
				Handler:       handler,
				Difficulty:    s.cfg.Difficulty,
				PluginManager: s.plugins,
			})
			if err != nil {
				return err
			}
			s.node = node
			defer node.Close()

			count, _ := s.db.Count()
			fmt.Printf("💾 %d msgs | 📡 %d peers | 👁  %s\n\n", count, node.PeerCount(), s.curView)
			printHelp()

			scanner := bufio.NewScanner(os.Stdin)
			fmt.Print("> ")
			for scanner.Scan() {
				text := strings.TrimSpace(scanner.Text())
				if text == "" {
					fmt.Print("> ")
					continue
				}
				if strings.HasPrefix(text, "/") {
					handleSlash(text, s)
					fmt.Print("> ")
					continue
				}
				m, err := msg.Create(s.id, text, s.cfg.Difficulty)
				if err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  create: %v\n> ", err)
					continue
				}
				if err := node.Publish(m); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  publish: %v\n> ", err)
					continue
				}
				s.db.Save(m)
				fmt.Print("> ")
			}
			return nil
		},
	}
}

func printHelp() {
	fmt.Println("Commands: <text> send | /peers | /history [n] | /view [name]")
	fmt.Println("  /views | /view-add <n> | /follow <pk> | /unfollow <pk>")
	fmt.Println("  /mute <pk> | /trust | /rules | /reload | /config")
	fmt.Println("  /plugins | /plugin-add <name> <type> <target> | /plugin-rm <name>")
	fmt.Println("  /abuse <pk> | /export | /help")
}

func handleSlash(text string, s *session) {
	parts := strings.Fields(text)
	cmd := parts[0]

	switch cmd {
	case "/help":
		printHelp()

	case "/peers":
		fmt.Printf("  Peers: %d\n", s.node.PeerCount())

	case "/history":
		limit := 10
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &limit)
		}
		msgs, _ := s.db.Latest(limit)
		for _, m := range msgs {
			ts := time.Unix(m.Timestamp, 0).Format("15:04:05")
			fmt.Printf("  [%s] %s: %s\n", ts, m.PubKey[:8], m.Content)
		}

	case "/view":
		if len(parts) < 2 {
			renderView(s)
			return
		}
		v := s.views.Get(parts[1])
		if v == nil {
			fmt.Printf("  Unknown view: %s\n", parts[1])
			return
		}
		s.curView = parts[1]
		fmt.Printf("  👁  Switched to: %s\n", s.curView)
		renderView(s)

	case "/views":
		for _, name := range s.views.List() {
			v := s.views.Get(name)
			marker := "  "
			if name == s.curView {
				marker = "▸ "
			}
			fmt.Printf("  %s%s (%s, trust≥%.1f)\n", marker, name, v.Strategy, v.MinTrust)
		}

	case "/view-add":
		if len(parts) < 2 {
			fmt.Println("  Usage: /view-add <name>")
			return
		}
		s.views.Set(view.View{
			Name: parts[1], Strategy: ranking.StrategyMixed, Limit: 50,
		})
		fmt.Printf("  ✅ Created: %s\n", parts[1])

	case "/follow":
		if len(parts) < 2 {
			fmt.Println("  Usage: /follow <pubkey>")
			return
		}
		s.node.Trust().Follow(parts[1])
		s.db.SetTrust(parts[1], 1.0)
		fmt.Printf("  ✅ Following %s\n", parts[1][:16])

	case "/unfollow":
		if len(parts) < 2 {
			fmt.Println("  Usage: /unfollow <pubkey>")
			return
		}
		s.node.Trust().Unfollow(parts[1])
		s.db.SetTrust(parts[1], 0.5)
		fmt.Printf("  🔓 Unfollowed %s\n", parts[1][:16])

	case "/mute":
		if len(parts) < 2 {
			fmt.Println("  Usage: /mute <pubkey>")
			return
		}
		s.node.Trust().Mute(parts[1])
		s.node.Filters().AddRule(filter.Rule{
			Name: "mute:" + parts[1][:8], Type: "blacklist_pubkey",
			Match: parts[1], Action: "block", Enabled: true,
		})
		s.db.SetTrust(parts[1], 0.0)
		fmt.Printf("  🔇 Muted %s\n", parts[1][:16])

	case "/trust":
		peers, _ := s.db.ListPeers()
		if len(peers) == 0 {
			fmt.Println("  No known peers")
			return
		}
		for _, p := range peers {
			score := s.node.Trust().Score(p.PubKey)
			icon := "  "
			if s.node.Trust().IsFollowing(p.PubKey) {
				icon = " 👤"
			}
			fmt.Printf("  %s: %.2f (%d msgs)%s\n", p.PubKey[:16], score, p.MsgCount, icon)
		}

	case "/rules":
		fmt.Printf("  Rules: %s\n", getRulesPath())

	case "/reload":
		if err := s.fe.LoadRules(getRulesPath()); err != nil {
			fmt.Printf("  ⚠️  %v\n", err)
		} else {
			fmt.Println("  ✅ Rules reloaded")
		}

	case "/config":
		out, _ := config.ExportConfig(s.cfg)
		fmt.Println(out)

	case "/plugins":
		plugins := s.plugins.List()
		if len(plugins) == 0 {
			fmt.Println("  No plugins registered. Use /plugin-add to add one.")
			return
		}
		for _, p := range plugins {
			status := "disabled"
			if p.Enabled {
				status = "enabled"
			}
			target := p.URL
			if target == "" {
				target = p.Command
			}
			fmt.Printf("  %s [%s] type=%s target=%s weight=%.1f\n",
				p.Name, status, p.Type, target, p.Weight)
		}

	case "/plugin-add":
		if len(parts) < 4 {
			fmt.Println("  Usage: /plugin-add <name> <http|script> <url_or_command>")
			return
		}
		pType := plugin.PluginType(parts[2])
		p := plugin.PluginDef{
			Name:    parts[1],
			Type:    pType,
			Enabled: true,
			Weight:  1.0,
		}
		if pType == plugin.TypeHTTP {
			p.URL = parts[3]
			p.Timeout = 5
		} else {
			p.Command = parts[3]
		}
		s.plugins.Register(p)
		fmt.Printf("  ✅ Plugin added: %s\n", parts[1])

	case "/plugin-rm":
		if len(parts) < 2 {
			fmt.Println("  Usage: /plugin-rm <name>")
			return
		}
		s.plugins.Unregister(parts[1])
		fmt.Printf("  🗑  Removed: %s\n", parts[1])

	case "/abuse":
		if len(parts) < 2 {
			// Show stats for all
			peers, _ := s.db.ListPeers()
			for _, p := range peers {
				sybil := s.node.Abuse()
				_ = sybil
				fmt.Printf("  %s: trust=%.2f msgs=%d\n", p.PubKey[:16], p.TrustScore, p.MsgCount)
			}
			return
		}
		// Show abuse analysis for specific pubkey
		pk := parts[1]
		trust := s.node.Trust().Score(pk)
		fmt.Printf("  PubKey: %s\n", pk[:24])
		fmt.Printf("  Trust score: %.3f\n", trust)
		fmt.Printf("  Following: %v\n", s.node.Trust().IsFollowing(pk))

	case "/export":
		vj, _ := s.views.ExportJSON()
		cj, _ := config.ExportConfig(s.cfg)
		exportPath := filepath.Join(dataDir, "export.json")
		export := fmt.Sprintf(`{"config":%s,"views":%s}`, cj, vj)
		os.WriteFile(exportPath, []byte(export), 0644)
		fmt.Printf("  📦 Exported to %s\n", exportPath)

	default:
		fmt.Printf("  Unknown: %s (try /help)\n", cmd)
	}
}

func renderView(s *session) {
	msgs, _ := s.db.Latest(500)
	trustScores := make(map[string]float64)
	peers, _ := s.db.ListPeers()
	for _, p := range peers {
		trustScores[p.PubKey] = s.node.Trust().Score(p.PubKey)
	}
	following := make(map[string]bool)
	for _, pk := range s.node.Trust().List() {
		following[pk] = true
	}

	ranked, err := s.views.Render(s.curView, msgs, trustScores, following)
	if err != nil {
		fmt.Printf("  ⚠️  %v\n", err)
		return
	}
	if len(ranked) == 0 {
		fmt.Println("  (empty)")
		return
	}
	fmt.Printf("  👁  %s (%d)\n", s.curView, len(ranked))
	for _, rm := range ranked {
		ts := time.Unix(rm.Message.Timestamp, 0).Format("15:04:05")
		fmt.Printf("  [%.3f] [%s] %s: %s\n", rm.Score, ts, rm.Message.PubKey[:8], rm.Message.Content)
	}
}

// --- Non-interactive commands ---

func idCmd() *cobra.Command {
	return &cobra.Command{
		Use: "id", Short: "Show identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := crypto.LoadOrGenerate(dataDir)
			if err != nil {
				return err
			}
			fmt.Printf("User ID: %s\nPubKey: %s\n", id.UserID, id.PubKeyHex())
			return nil
		},
	}
}

func sendCmd() *cobra.Command {
	return &cobra.Command{
		Use: "send [message]", Short: "Send a message",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			node, err := dmsgnet.NewNode(dmsgnet.Config{
				ListenAddr: s.cfg.ListenAddr, Bootstrap: s.cfg.Bootstrap,
				Rendezvous: s.cfg.Rendezvous, Difficulty: s.cfg.Difficulty,
				PluginManager: s.plugins,
			})
			if err != nil {
				return err
			}
			defer node.Close()
			content := strings.Join(args, " ")
			m, err := msg.Create(s.id, content, s.cfg.Difficulty)
			if err != nil {
				return err
			}
			node.Publish(m)
			s.db.Save(m)
			fmt.Printf("✅ Sent: %s\n", m.ID[:16])
			return nil
		},
	}
}

func peersCmd() *cobra.Command {
	return &cobra.Command{
		Use: "peers", Short: "Show peers",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			node, err := dmsgnet.NewNode(dmsgnet.Config{
				ListenAddr: s.cfg.ListenAddr, Bootstrap: s.cfg.Bootstrap,
				Rendezvous: s.cfg.Rendezvous,
			})
			if err != nil {
				return err
			}
			defer node.Close()
			time.Sleep(3 * time.Second)
			fmt.Printf("Peers: %d\n", node.PeerCount())
			return nil
		},
	}
}

func followCmd() *cobra.Command {
	return &cobra.Command{
		Use: "follow <pubkey>", Short: "Follow a pubkey",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			s.db.SetTrust(args[0], 1.0)
			fmt.Printf("✅ Following %s\n", args[0][:16])
			return nil
		},
	}
}

func muteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "mute <pubkey>", Short: "Mute a pubkey",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			s.db.SetTrust(args[0], 0.0)
			rp := getRulesPath()
			fe := filter.NewEngine()
			filter.SaveDefaultRules(rp)
			fe.LoadRules(rp)
			fe.AddRule(filter.Rule{
				Name: "mute:" + args[0][:8], Type: "blacklist_pubkey",
				Match: args[0], Action: "block", Enabled: true,
			})
			fe.SaveRules(rp)
			fmt.Printf("🔇 Muted %s\n", args[0][:16])
			return nil
		},
	}
}

func trustCmd() *cobra.Command {
	return &cobra.Command{
		Use: "trust", Short: "Show trust scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			peers, _ := s.db.ListPeers()
			for _, p := range peers {
				fmt.Printf("  %s  trust=%.2f  msgs=%d\n", p.PubKey[:24], p.TrustScore, p.MsgCount)
			}
			return nil
		},
	}
}

func rulesCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rules", Short: "Show rules path",
		Run: func(cmd *cobra.Command, args []string) {
			rp := getRulesPath()
			filter.SaveDefaultRules(rp)
			fmt.Printf("Rules: %s\n", rp)
		},
	}
}

func historyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "history", Short: "Show recent messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := loadSession()
			if err != nil {
				return err
			}
			defer s.db.Close()
			msgs, _ := s.db.Latest(20)
			for _, m := range msgs {
				ts := time.Unix(m.Timestamp, 0).Format("2006-01-02 15:04:05")
				fmt.Printf("[%s] %s: %s\n", ts, m.PubKey[:16], m.Content)
			}
			return nil
		},
	}
}

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use: "config", Short: "Show configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(dataDir)
			if err != nil {
				return err
			}
			out, _ := config.ExportConfig(cfg)
			fmt.Println(out)
			return nil
		},
	}
}

func viewCmd() *cobra.Command {
	return &cobra.Command{
		Use: "views", Short: "List views",
		RunE: func(cmd *cobra.Command, args []string) error {
			vm, err := view.NewManager(dataDir)
			if err != nil {
				return err
			}
			for _, name := range vm.List() {
				v := vm.Get(name)
				fmt.Printf("  %-15s %s trust≥%.1f limit=%d\n", name, v.Strategy, v.MinTrust, v.Limit)
			}
			return nil
		},
	}
}

func pluginCmd() *cobra.Command {
	return &cobra.Command{
		Use: "plugins", Short: "List plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			pm, err := plugin.NewManager(dataDir)
			if err != nil {
				return err
			}
			plugins := pm.List()
			if len(plugins) == 0 {
				fmt.Println("No plugins. Add to ~/.dmsg/plugins.json")
				return nil
			}
			for _, p := range plugins {
				status := "off"
				if p.Enabled {
					status = "on"
				}
				target := p.URL
				if target == "" {
					target = p.Command
				}
				data, _ := json.MarshalIndent(p, "  ", "  ")
				fmt.Printf("  %s [%s] %s\n", p.Name, status, target)
				_ = data
			}
			return nil
		},
	}
}
