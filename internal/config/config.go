// Package config, kullanıcı ayarlarını JSON dosyasında saklar.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/UmutKDev/slipstream/internal/desync"
	"github.com/UmutKDev/slipstream/internal/domains"
	"github.com/UmutKDev/slipstream/internal/profile"
)

// Config, kalıcı ayarlardır.
type Config struct {
	Profile     string           `json:"profile"`
	Custom      *desync.Strategy `json:"custom,omitempty"` // otomatik bulmanın ya da kullanıcının seçtiği özel strateji
	DNS         []string         `json:"dns"`
	Mode        domains.Mode     `json:"mode"`
	Domains     []string         `json:"domains"`
	Listen      string           `json:"listen"`
	SystemProxy bool             `json:"systemProxy"`
}

// Default, ilk açılış ayarlarıdır.
func Default() Config {
	p := profile.All[profile.Default]
	return Config{
		Profile: p.ID,
		DNS:     p.DNS,
		Mode:    domains.ModeAll,
		Domains: append([]string(nil), domains.DefaultBlocked...),
		Listen:  "127.0.0.1:8881",
	}
}

// DefaultPath, macOS'ta ~/Library/Application Support/Slipstream/config.json yoludur.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "Slipstream", "config.json")
}

// Strategy, geçerli stratejiyi döndürür: önce özel, sonra profil.
func (c Config) Strategy() desync.Strategy {
	if c.Profile == "custom" && c.Custom != nil {
		return *c.Custom
	}
	if p, ok := profile.All[c.Profile]; ok {
		return p.Strategy
	}
	return profile.All[profile.Default].Strategy
}

// Store, ayarları eşzamanlı güvenli biçimde okuyup yazar.
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

// Load, dosyadan okur. Dosya yoksa varsayılanlarla başlar.
func Load(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, err
	}
	return s, nil
}

// Get, ayarların kopyasını döndürür.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update, ayarları değiştirir ve diske yazar.
func (s *Store) Update(fn func(*Config)) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	if s.path == "" {
		return s.cfg, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return s.cfg, err
	}
	b, _ := json.MarshalIndent(s.cfg, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return s.cfg, err
	}
	return s.cfg, os.Rename(tmp, s.path)
}
