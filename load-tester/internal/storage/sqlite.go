package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Hitesh-s0lanki/system-arch/load-tester/internal/model"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

var ErrNotFound = errors.New("test not found")

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, status TEXT NOT NULL, created TEXT NOT NULL, payload BLOB NOT NULL);`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Save(j model.Job) error {
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	_, e = s.db.Exec(`INSERT INTO jobs VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,payload=excluded.payload`, j.ID, j.Status, j.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), b)
	return e
}
func (s *Store) Get(id string) (model.Job, error) {
	var b []byte
	var j model.Job
	e := s.db.QueryRow(`SELECT payload FROM jobs WHERE id=?`, id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	if e != nil {
		return j, e
	}
	e = json.Unmarshal(b, &j)
	return j, e
}
func (s *Store) List(limit, offset int) ([]model.Job, error) {
	return s.query(`SELECT payload FROM jobs ORDER BY created DESC LIMIT ? OFFSET ?`, limit, offset)
}
func (s *Store) Pending() ([]model.Job, error) {
	return s.query(`SELECT payload FROM jobs WHERE status IN ('queued','running','cancelling') ORDER BY created`)
}
func (s *Store) query(q string, args ...any) ([]model.Job, error) {
	rows, e := s.db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Job{}
	for rows.Next() {
		var b []byte
		var j model.Job
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &j); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
