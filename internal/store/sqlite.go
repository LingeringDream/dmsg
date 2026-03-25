package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"dmsg/internal/msg"
)

// Store is a SQLite-backed message store with pruning and hot/cold support.
type Store struct {
	db     *sql.DB
	dbPath string
}

// Config holds store configuration.
type Config struct {
	Path    string
	MaxRows int           // max rows before pruning (0 = unlimited)
	MaxAge  time.Duration // max age before pruning (0 = unlimited)
}

// Open opens (or creates) a SQLite database.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db, dbPath: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id         TEXT PRIMARY KEY,
			pubkey     TEXT NOT NULL,
			content    TEXT NOT NULL,
			timestamp  INTEGER NOT NULL,
			nonce      INTEGER NOT NULL,
			signature  TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE INDEX IF NOT EXISTS idx_msg_pubkey ON messages(pubkey);
		CREATE INDEX IF NOT EXISTS idx_msg_ts ON messages(timestamp);
		CREATE INDEX IF NOT EXISTS idx_msg_pubkey_ts ON messages(pubkey, timestamp);

		CREATE TABLE IF NOT EXISTS peers (
			pubkey      TEXT PRIMARY KEY,
			first_seen  INTEGER NOT NULL,
			last_seen   INTEGER NOT NULL,
			msg_count   INTEGER NOT NULL DEFAULT 0,
			trust_score REAL NOT NULL DEFAULT 0.5
		);
	`)
	return err
}

// Save stores a message. Ignores duplicates (by ID).
func (s *Store) Save(m *msg.Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT OR IGNORE INTO messages (id, pubkey, content, timestamp, nonce, signature)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.PubKey, m.Content, m.Timestamp, m.Nonce, m.Signature,
	)
	if err != nil {
		return err
	}

	// Upsert peer stats
	now := time.Now().Unix()
	_, err = tx.Exec(
		`INSERT INTO peers (pubkey, first_seen, last_seen, msg_count)
		 VALUES (?, ?, ?, 1)
		 ON CONFLICT(pubkey) DO UPDATE SET
		   last_seen = excluded.last_seen,
		   msg_count = msg_count + 1`,
		m.PubKey, now, now,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Latest returns the most recent messages, newest first.
func (s *Store) Latest(limit int) ([]*msg.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, pubkey, content, timestamp, nonce, signature
		 FROM messages ORDER BY timestamp DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ByUser returns messages from a specific pubkey.
func (s *Store) ByUser(pubkey string, limit int) ([]*msg.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, pubkey, content, timestamp, nonce, signature
		 FROM messages WHERE pubkey = ? ORDER BY timestamp DESC LIMIT ?`,
		pubkey, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Since returns messages newer than a given timestamp.
func (s *Store) Since(ts int64, limit int) ([]*msg.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, pubkey, content, timestamp, nonce, signature
		 FROM messages WHERE timestamp > ? ORDER BY timestamp ASC LIMIT ?`,
		ts, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Count returns total message count.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n)
	return n, err
}

// PruneByAge removes messages older than maxAge.
func (s *Store) PruneByAge(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	res, err := s.db.Exec(`DELETE FROM messages WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneByCount keeps only the newest maxRows messages.
func (s *Store) PruneByCount(maxRows int) (int64, error) {
	res, err := s.db.Exec(`
		DELETE FROM messages WHERE id IN (
			SELECT id FROM messages ORDER BY timestamp ASC
			LIMIT MAX(0, (SELECT COUNT(*) FROM messages) - ?)
		)`, maxRows)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AutoPrune runs both age and count pruning.
func (s *Store) AutoPrune(cfg Config) (int64, error) {
	var total int64
	if cfg.MaxAge > 0 {
		n, _ := s.PruneByAge(cfg.MaxAge)
		total += n
	}
	if cfg.MaxRows > 0 {
		n, _ := s.PruneByCount(cfg.MaxRows)
		total += n
	}
	return total, nil
}

// GetTrust returns the trust score for a pubkey.
func (s *Store) GetTrust(pubkey string) float64 {
	var score float64
	err := s.db.QueryRow(`SELECT trust_score FROM peers WHERE pubkey = ?`, pubkey).Scan(&score)
	if err != nil {
		return 0.5 // default neutral
	}
	return score
}

// SetTrust updates the trust score for a pubkey.
func (s *Store) SetTrust(pubkey string, score float64) error {
	_, err := s.db.Exec(
		`UPDATE peers SET trust_score = ? WHERE pubkey = ?`,
		score, pubkey,
	)
	return err
}

// ListPeers returns all known peers with their stats.
type PeerStat struct {
	PubKey     string
	FirstSeen  int64
	LastSeen   int64
	MsgCount   int
	TrustScore float64
}

func (s *Store) ListPeers() ([]PeerStat, error) {
	rows, err := s.db.Query(`SELECT pubkey, first_seen, last_seen, msg_count, trust_score FROM peers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []PeerStat
	for rows.Next() {
		var p PeerStat
		if err := rows.Scan(&p.PubKey, &p.FirstSeen, &p.LastSeen, &p.MsgCount, &p.TrustScore); err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func scanMessages(rows *sql.Rows) ([]*msg.Message, error) {
	var msgs []*msg.Message
	for rows.Next() {
		var m msg.Message
		if err := rows.Scan(&m.ID, &m.PubKey, &m.Content, &m.Timestamp, &m.Nonce, &m.Signature); err != nil {
			return nil, err
		}
		msgs = append(msgs, &m)
	}
	return msgs, rows.Err()
}
