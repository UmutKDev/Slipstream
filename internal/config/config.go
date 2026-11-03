// Package config, kullanıcı ayarlarını JSON dosyasında saklar.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/UmutKDev/Slipstream/internal/desync"
	"github.com/UmutKDev/Slipstream/internal/domains"
	"github.com/UmutKDev/Slipstream/internal/profile"
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
	ProxyMode   string           `json:"proxyMode"` // "global" ya da "pac"
	AutoLearn   bool             `json:"autoLearn"` // çalışmayan sitede sıradaki stratejiyi dene ve hatırla
	AdBlock     bool             `json:"adBlock"`   // reklam ve izleyici engelleyici
	AdAllow     []string         `json:"adAllow"`   // reklam engelleyicinin dokunmayacağı alan adları
}

// Default, ilk açılış ayarlarıdır.
func Default() Config {
	p := profile.All[profile.Default]
	return Config{
		Profile:   p.ID,
		DNS:       p.DNS,
		Mode:      domains.ModeAll,
		Domains:   append([]string(nil), domains.DefaultBlocked...),
		Listen:    "127.0.0.1:8881",
		ProxyMode: "global",
		AutoLearn: true,
		AdBlock:   true,
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

// Dir, ayar dosyasının bulunduğu klasördür; öğrenilen stratejiler ve
// reklam listeleri de burada saklanır.
func (s *Store) Dir() string {
	if s.path == "" {
		return os.TempDir()
	}
	return filepath.Dir(s.path)
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

// LoadLearned, öğrenilen site stratejilerini okur.
func LoadLearned(dir string) map[string]desync.Strategy {
	m := map[string]desync.Strategy{}
	b, err := os.ReadFile(filepath.Join(dir, "learned.json"))
	if err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

// SaveLearned, öğrenilen site stratejilerini yazar.
func SaveLearned(dir string, m map[string]desync.Strategy) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := filepath.Join(dir, "learned.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "learned.json"))
}
