package tracker

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"os"
	"strings"
	"time"
)

type Store struct {
	db   *sql.DB
	file string
}

func OpenStore(ctx context.Context, file string) (*Store, error) {
	host := os.Getenv("TIDB_HOST")
	if host == "" {
		return &Store{file: file}, nil
	}
	for _, key := range []string{"TIDB_USER", "TIDB_PASSWORD", "TIDB_DATABASE"} {
		if os.Getenv(key) == "" {
			return nil, errors.New("missing TiDB connection secret")
		}
	}
	if e := mysql.RegisterTLSConfig("tidb", &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}); e != nil {
		return nil, e
	}
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("TIDB_USER")
	cfg.Passwd = os.Getenv("TIDB_PASSWORD")
	cfg.Net = "tcp"
	port := os.Getenv("TIDB_PORT")
	if port == "" {
		port = "4000"
	}
	cfg.Addr = host + ":" + port
	cfg.DBName = os.Getenv("TIDB_DATABASE")
	cfg.TLSConfig = "tidb"
	cfg.Timeout = 20 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		return nil, errors.New("database configuration failed")
	}
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(3 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, errors.New("TiDB connection failed; check secrets, network access, and free quota")
	}
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tracker_state (id INT PRIMARY KEY, payload JSON NOT NULL)`)
	if e != nil {
		db.Close()
		return nil, errors.New("could not create tracker_state table")
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() {
	if s.db != nil {
		s.db.Close()
	}
}
func (s *Store) Load(ctx context.Context) (State, error) {
	state := NewState()
	var b []byte
	var e error
	if s.db != nil {
		e = s.db.QueryRowContext(ctx, "SELECT payload FROM tracker_state WHERE id=1").Scan(&b)
		if errors.Is(e, sql.ErrNoRows) {
			return state, nil
		}
	} else {
		b, e = os.ReadFile(s.file)
		if errors.Is(e, os.ErrNotExist) {
			return state, nil
		}
	}
	if e != nil {
		return state, errors.New("could not read persistent job history")
	}
	if e = json.Unmarshal(b, &state); e != nil {
		return state, errors.New("job history is not valid JSON")
	}
	if state.Jobs == nil {
		state.Jobs = map[string]Job{}
	}
	if state.Notified == nil {
		state.Notified = map[string]bool{}
	}
	return state, nil
}
func (s *Store) Save(ctx context.Context, state State) error {
	b, e := json.Marshal(state)
	if e != nil {
		return e
	}
	if s.db != nil {
		_, e = s.db.ExecContext(ctx, "INSERT INTO tracker_state(id,payload) VALUES(1,?) ON DUPLICATE KEY UPDATE payload=VALUES(payload)", string(b))
		if e != nil {
			return errors.New("could not save TiDB job history")
		}
		return nil
	}
	if strings.TrimSpace(s.file) == "" {
		return errors.New("state file is required")
	}
	return WriteJSON(s.file, state)
}
