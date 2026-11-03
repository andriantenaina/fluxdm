// Package store persists downloads, queues and settings in SQLite (pure Go driver).
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Item kinds.
const (
	KindHTTP    = "http"    // direct HTTP/HTTPS file (multi-connection)
	KindFTP     = "ftp"     // FTP file
	KindHLS     = "hls"     // HLS playlist (.m3u8 -> .ts / .mp4)
	KindDASH    = "dash"    // DASH manifest (.mpd)
	KindYtdlp   = "ytdlp"   // page URL handled by yt-dlp
	KindTorrent = "torrent" // magnet or .torrent
	KindHoster  = "hoster"  // video host embed page (Voe, Doodstream, Uqload, Vidmoly...)
)

// Item statuses.
const (
	StatusQueued      = "queued"
	StatusDownloading = "downloading"
	StatusPaused      = "paused"
	StatusCompleted   = "completed"
	StatusError       = "error"
	StatusProcessing  = "processing" // post-processing (merge/remux/antivirus)
)

// Item is one entry of the download list.
type Item struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	URL         string            `json:"url"`
	PageURL     string            `json:"pageUrl,omitempty"`
	Title       string            `json:"title,omitempty"`
	FileName    string            `json:"fileName"`
	Dir         string            `json:"dir"`
	Path        string            `json:"path,omitempty"`
	Category    string            `json:"category"`
	Queue       string            `json:"queue"`
	Status      string            `json:"status"`
	Size        int64             `json:"size"`
	Downloaded  int64             `json:"downloaded"`
	Error       string            `json:"error,omitempty"`
	Description string            `json:"description,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Options     map[string]any    `json:"options,omitempty"`
	State       json.RawMessage   `json:"-"` // engine-specific resume state
	CreatedAt   time.Time         `json:"createdAt"`
	CompletedAt *time.Time        `json:"completedAt,omitempty"`
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// ErrNotFound is returned when an item does not exist.
var ErrNotFound = errors.New("introuvable")

// Open opens (and creates) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  data TEXT NOT NULL,
  state BLOB,
  status TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS items_status ON items(status);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`)
	return err
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SaveItem inserts or updates an item (including its resume state).
func (s *Store) SaveItem(it *Item) error {
	data, err := json.Marshal(it)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO items(id,data,state,status,created_at) VALUES(?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET data=excluded.data, state=excluded.state, status=excluded.status`,
		it.ID, string(data), []byte(it.State), it.Status, it.CreatedAt.Unix())
	return err
}

// DeleteItem removes an item.
func (s *Store) DeleteItem(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM items WHERE id=?`, id)
	return err
}

// GetItem loads one item.
func (s *Store) GetItem(id string) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT data, state FROM items WHERE id=?`, id)
	var data string
	var state []byte
	if err := row.Scan(&data, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	it := &Item{}
	if err := json.Unmarshal([]byte(data), it); err != nil {
		return nil, err
	}
	it.State = state
	return it, nil
}

// ListItems returns all items, oldest first.
func (s *Store) ListItems() ([]*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT data, state FROM items ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		var data string
		var state []byte
		if err := rows.Scan(&data, &state); err != nil {
			return nil, err
		}
		it := &Item{}
		if err := json.Unmarshal([]byte(data), it); err != nil {
			continue
		}
		it.State = state
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetSetting decodes the JSON value stored under key into v. Returns false if absent.
func (s *Store) GetSetting(key string, v any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as JSON under key.
func (s *Store) SetSetting(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(raw))
	return err
}
