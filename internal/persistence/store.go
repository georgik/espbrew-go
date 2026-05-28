package persistence

import (
	"fmt"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	defaultTimeout = 5 * time.Second
)

type Store struct {
	db *bolt.DB
	mu sync.RWMutex
}

type Config struct {
	Path         string
	Timeout      time.Duration
	NoSync       bool
	NoGrowSync   bool
	ReadOnly     bool
	FreelistType string
}

func DefaultConfig(path string) *Config {
	return &Config{
		Path:         path,
		Timeout:      defaultTimeout,
		FreelistType: "hash",
	}
}

func Open(cfg *Config) (*Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config required")
	}

	opts := &bolt.Options{
		Timeout:    cfg.Timeout,
		NoGrowSync: cfg.NoGrowSync,
		ReadOnly:   cfg.ReadOnly,
	}

	if cfg.FreelistType == "hash" {
		opts.FreelistType = bolt.FreelistMapType
	} else {
		opts.FreelistType = bolt.FreelistArrayType
	}

	db, err := bolt.Open(cfg.Path, 0600, opts)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		return initBuckets(tx)
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) Backup(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(path, 0600)
	})
}

func (s *Store) Stats() bolt.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db.Stats()
}
