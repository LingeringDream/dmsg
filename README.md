# dmsg — Decentralized P2P Messaging

A decentralized messaging network built on libp2p. No central servers. No registration. Just keys and peers.

## Quick Start

```bash
chmod +x setup.sh && ./setup.sh

# Terminal 1: bootstrap node
./bin/dmsg start

# Terminal 2: connect
./bin/dmsg start --listen /ip4/0.0.0.0/tcp/4002 \
  --bootstrap /ip4/127.0.0.1/tcp/4001/p2p/<PEER_ID>
```

## Full Processing Pipeline

```
receive → dedup → rate limit → replay → PoW → signature
  → trust record → forward score → abuse detect → filter → plugin → deliver
```

## Commands (CLI + Interactive)

| CLI Command | Interactive | Description |
|-------------|-------------|-------------|
| `dmsg start` | — | Start node (interactive shell) |
| `dmsg id` | — | Show identity |
| `dmsg send "x"` | `<text>` | Send message |
| `dmsg peers` | `/peers` | Show peers |
| `dmsg follow <pk>` | `/follow <pk>` | Follow (trust=1.0) |
| `dmsg mute <pk>` | `/mute <pk>` | Mute (trust=0.0, block) |
| `dmsg trust` | `/trust` | Show trust scores |
| `dmsg history` | `/history [n]` | Recent messages |
| `dmsg views` | `/views` | List views |
| — | `/view [name]` | Switch view / render |
| — | `/view-add <n>` | Create view |
| `dmsg rules` | `/rules` | Rules file path |
| — | `/reload` | Reload filter rules |
| `dmsg config` | `/config` | Show config |
| `dmsg plugins` | `/plugins` | List plugins |
| — | `/plugin-add <n> <type> <target>` | Add plugin |
| — | `/plugin-rm <n>` | Remove plugin |
| — | `/abuse [pk]` | Abuse analysis |
| — | `/export` | Export config+views |

## Modules (30 files, ~4500 lines)

| Module | Files | Description |
|--------|-------|-------------|
| `crypto` | identity.go, multi.go | Ed25519 + multi-identity |
| `net` | node.go, discovery.go, addr.go | libp2p + GossipSub + DHT |
| `msg` | message.go, pow.go, ratelimit.go, dedup.go, replay.go | Message model + anti-abuse primitives |
| `trust` | engine.go | Direct + indirect trust (α^depth decay) |
| `filter` | engine.go | Keyword/regex/blacklist/allowlist |
| `abuse` | detector.go | Content fingerprinting + Sybil + anomaly |
| `plugin` | manager.go | HTTP webhook + script plugins |
| `ranking` | engine.go | Time/trust/hot/mixed strategies |
| `view` | manager.go | Multi-view with filter+rank combos |
| `store` | sqlite.go | SQLite WAL + peer stats + pruning |
| `config` | config.go | JSON config + import/export |
| `ipfs` | store.go | Optional content-addressed storage |

## Anti-Abuse System

### Layer 1: Message-Level
- **PoW** — SHA256 leading zeros, dynamic difficulty
- **Rate Limit** — Token bucket per pubkey (10/min, burst 20)
- **Dedup** — LRU cache (100K entries)
- **Replay** — Timestamp window + signature dedup

### Layer 2: Behavioral
- **Trust Engine** — `trust = direct + α × Σ(indirect × decay^depth)`
- **Forward Score** — `trust / frequency`
- **Sybil Detection** — Account age + timing regularity + content uniformity + burst creation
- **Anomaly Detection** — Rate anomaly per pubkey

### Layer 3: Content
- **Fingerprinting** — K-shingle near-duplicate detection (Jaccard similarity)
- **Filter Engine** — Keyword, regex, blacklist, allowlist, content-type
- **Plugin System** — External HTTP/script rules with weighted scoring

## Trust Model

```
trust(A,B) = direct + 0.5 × Σ(indirect × 0.5^depth)
forward_score = trust / frequency

Direct:  follow=1.0, default=0.5, mute=0.0
Penalty: burst(>10 msg/10s) → score × 0.3
Sybil:   new account + regular timing + uniform content → suspicion
```

## Ranking Strategies

| Strategy | Formula | Use Case |
|----------|---------|----------|
| `time` | timestamp | Latest |
| `trust` | trust×1e12 + timestamp | Quality |
| `hot` | log(forwards) + timestamp | Trending |
| `mixed` | trust×w + time_decay + hot | Default |

## Views (Pre-configured)

- **All** — Mixed, no trust filter
- **Following** — Time, followed only
- **High Trust** — Mixed, trust ≥ 0.7
- **Trending** — Hot, trust ≥ 0.3
- **Latest** — Time, no filter
- Custom: create with `/view-add`

## Plugin System

Plugins extend filtering with external rules. Two types:

### HTTP Plugin
```json
{
  "name": "ml-spam",
  "type": "http",
  "url": "http://localhost:8080/check",
  "timeout": 5,
  "weight": 1.5,
  "enabled": true
}
```
POST request receives `{"id","pubkey","content","timestamp"}`, expects `{"action","score","reason"}`.

### Script Plugin
```json
{
  "name": "custom-filter",
  "type": "script",
  "command": "/path/to/filter.sh",
  "weight": 1.0,
  "enabled": true
}
```
Receives JSON on stdin, outputs JSON or plain score (0-1).

## Configuration

`~/.dmsg/config.json`:
```json
{
  "listen_addr": "/ip4/0.0.0.0/tcp/4001",
  "rendezvous": "dmsg-v1",
  "difficulty": 8,
  "rate_per_minute": 10,
  "rate_burst": 20,
  "max_messages": 100000,
  "max_age": "72h0m0s",
  "trust_alpha": 0.5,
  "trust_decay": 0.5,
  "default_view": "All",
  "page_size": 50,
  "max_msg_age": "24h0m0s"
}
```

## IPFS Integration (Optional)

Large content can be stored off-chain via IPFS:
- `ContentRef` — inline (small) or CID (large)
- Local content-addressed cache (SHA256)
- Falls back to local if IPFS unavailable
- `~/.dmsg/ipfs-cache/` for local storage

## Architecture

```
Identity (Ed25519 + Multi-identity)
    ↓
Networking (libp2p + GossipSub + Kademlia DHT)
    ↓
Data (Message model)
    ↓
Processing Pipeline (9 stages)
    ↓
Ranking (4 strategies)
    ↓
Views (5 presets + custom)
    ↓
Plugins (HTTP + Script)
    ↓
Storage (SQLite WAL)
    ↓
Config (JSON)
    ↓
CLI (12 commands + 18 interactive)
```
