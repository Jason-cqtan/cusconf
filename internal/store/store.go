// Package store persists sources, profiles and subscription tokens in SQLite.
//
// Entities are stored as JSON documents keyed by (kind, id); the schema stays
// trivial while the models evolve.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/cusconf/cusconf/internal/capability"
	"github.com/cusconf/cusconf/internal/node"
	"github.com/cusconf/cusconf/internal/profile"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS docs (
  kind TEXT NOT NULL, id TEXT NOT NULL, data TEXT NOT NULL, updated_at INTEGER NOT NULL,
  PRIMARY KEY (kind, id)
);
CREATE TABLE IF NOT EXISTS node_cache (
  source_id TEXT PRIMARY KEY, nodes TEXT NOT NULL, fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS access_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT, token TEXT NOT NULL, at INTEGER NOT NULL,
  ip TEXT, user_agent TEXT, status INTEGER
);
CREATE INDEX IF NOT EXISTS access_log_token ON access_log(token, at);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS node_tests (key TEXT PRIMARY KEY, data TEXT NOT NULL, tested_at INTEGER NOT NULL);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func NewID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewToken returns a long random subscription token.
func NewToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// --- generic docs ---

func put[T any](s *Store, kind, id string, v T) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO docs(kind,id,data,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at`,
		kind, id, string(b), time.Now().Unix())
	return err
}

func get[T any](s *Store, kind, id string) (*T, error) {
	var data string
	err := s.db.QueryRow(`SELECT data FROM docs WHERE kind=? AND id=?`, kind, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var v T
	return &v, json.Unmarshal([]byte(data), &v)
}

func list[T any](s *Store, kind string) ([]T, error) {
	rows, err := s.db.Query(`SELECT data FROM docs WHERE kind=? ORDER BY rowid`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func del(s *Store, kind, id string) error {
	res, err := s.db.Exec(`DELETE FROM docs WHERE kind=? AND id=?`, kind, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- sources ---

type SubInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // url | manual
	URL       string `json:"url,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Content   string `json:"content,omitempty"` // manual nodes (any supported format)
	// RefreshInterval in seconds; 0 disables scheduled refresh.
	RefreshInterval int  `json:"refreshInterval"`
	Enabled         bool `json:"enabled"`
	// SelfHosted marks the source's nodes as your own servers: they are kept out of
	// region-based groups and can be grouped on their own.
	SelfHosted bool `json:"selfHosted"`

	// Status, maintained by the fetcher.
	FetchedAt int64    `json:"fetchedAt,omitempty"`
	LastError string   `json:"lastError,omitempty"`
	NodeCount int      `json:"nodeCount"`
	Format    string   `json:"format,omitempty"`
	Info      *SubInfo `json:"info,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func (s *Store) PutSource(v *Source) error            { return put(s, "source", v.ID, v) }
func (s *Store) GetSource(id string) (*Source, error) { return get[Source](s, "source", id) }
func (s *Store) ListSources() ([]Source, error)       { return list[Source](s, "source") }
func (s *Store) DeleteSource(id string) error {
	s.db.Exec(`DELETE FROM node_cache WHERE source_id=?`, id)
	return del(s, "source", id)
}

func (s *Store) PutNodes(sourceID string, nodes []*node.Node) error {
	b, err := json.Marshal(nodes)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO node_cache(source_id,nodes,fetched_at) VALUES(?,?,?)
		ON CONFLICT(source_id) DO UPDATE SET nodes=excluded.nodes, fetched_at=excluded.fetched_at`,
		sourceID, string(b), time.Now().Unix())
	return err
}

func (s *Store) GetNodes(sourceID string) ([]*node.Node, error) {
	var data string
	err := s.db.QueryRow(`SELECT nodes FROM node_cache WHERE source_id=?`, sourceID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var nodes []*node.Node
	return nodes, json.Unmarshal([]byte(data), &nodes)
}

// AllNodes returns cached nodes of all enabled sources, in source order.
func (s *Store) AllNodes() ([]*node.Node, error) {
	srcs, err := s.ListSources()
	if err != nil {
		return nil, err
	}
	var out []*node.Node
	for _, src := range srcs {
		if !src.Enabled {
			continue
		}
		ns, err := s.GetNodes(src.ID)
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			n.Source = src.ID
			n.SelfHosted = src.SelfHosted
		}
		out = append(out, ns...)
	}
	return out, nil
}

// --- profiles ---

func (s *Store) PutProfile(v *profile.Profile) error { return put(s, "profile", v.ID, v) }
func (s *Store) GetProfile(id string) (*profile.Profile, error) {
	return get[profile.Profile](s, "profile", id)
}
func (s *Store) ListProfiles() ([]profile.Profile, error) { return list[profile.Profile](s, "profile") }
func (s *Store) DeleteProfile(id string) error            { return del(s, "profile", id) }

// --- tokens ---

type Token struct {
	Token     string            `json:"token"`
	Name      string            `json:"name"`
	ProfileID string            `json:"profileId"`
	Target    capability.Target `json:"target"`
	ExpiresAt int64             `json:"expiresAt,omitempty"` // unix seconds; 0 = never
	MaxUses   int               `json:"maxUses,omitempty"`   // 0 = unlimited
	Revoked   bool              `json:"revoked"`
	CreatedAt int64             `json:"createdAt"`

	Uses       int    `json:"uses"`
	LastUsedAt int64  `json:"lastUsedAt,omitempty"`
	LastIP     string `json:"lastIp,omitempty"`
	LastUA     string `json:"lastUa,omitempty"`
}

// Usable reports whether the token may serve a config now.
func (t *Token) Usable(now time.Time) error {
	switch {
	case t.Revoked:
		return errors.New("revoked")
	case t.ExpiresAt > 0 && now.Unix() > t.ExpiresAt:
		return errors.New("expired")
	case t.MaxUses > 0 && t.Uses >= t.MaxUses:
		return errors.New("usage limit reached")
	}
	return nil
}

func (s *Store) PutToken(v *Token) error            { return put(s, "token", v.Token, v) }
func (s *Store) GetToken(id string) (*Token, error) { return get[Token](s, "token", id) }
func (s *Store) ListTokens() ([]Token, error)       { return list[Token](s, "token") }
func (s *Store) DeleteToken(id string) error        { return del(s, "token", id) }

type Access struct {
	At        int64  `json:"at"`
	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`
	Status    int    `json:"status"`
}

func (s *Store) LogAccess(token string, a Access) error {
	_, err := s.db.Exec(`INSERT INTO access_log(token,at,ip,user_agent,status) VALUES(?,?,?,?,?)`,
		token, a.At, a.IP, a.UserAgent, a.Status)
	return err
}

func (s *Store) ListAccess(token string, limit int) ([]Access, error) {
	rows, err := s.db.Query(`SELECT at,ip,user_agent,status FROM access_log WHERE token=? ORDER BY at DESC LIMIT ?`, token, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Access{}
	for rows.Next() {
		var a Access
		if err := rows.Scan(&a.At, &a.IP, &a.UserAgent, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- settings ---

func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// --- node test results (stored as opaque JSON, keyed by node.Key()) ---

func (s *Store) PutTest(key string, data []byte, at int64) error {
	_, err := s.db.Exec(`INSERT INTO node_tests(key,data,tested_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET data=excluded.data, tested_at=excluded.tested_at`, key, string(data), at)
	return err
}

func (s *Store) Tests() (map[string][]byte, error) {
	rows, err := s.db.Query(`SELECT key, data FROM node_tests`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var k, d string
		if err := rows.Scan(&k, &d); err != nil {
			return nil, err
		}
		out[k] = []byte(d)
	}
	return out, rows.Err()
}
