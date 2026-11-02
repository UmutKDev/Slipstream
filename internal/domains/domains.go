// Package domains, atlatmanın hangi alan adlarına uygulanacağını belirler.
package domains

import (
	"bufio"
	"io"
	"strings"
	"sync"
)

// Mode, eşleştirme kipidir.
type Mode string

const (
	ModeAll  Mode = "all"  // her bağlantıya uygula
	ModeList Mode = "list" // yalnız listedeki alan adlarına uygula
)

// DefaultBlocked, Türkiye'de sık engellenen ya da kısıtlanan hizmetlerin
// başlangıç listesidir. Kullanıcı arayüzden düzenleyebilir.
var DefaultBlocked = []string{
	"discord.com", "discord.gg", "discordapp.com", "discordapp.net",
	"discord.media", "discordcdn.com", "discord.dev", "discordstatus.com",
	"roblox.com", "rbxcdn.com", "roblox.cn",
	"wattpad.com", "imgur.com", "pastebin.com", "archive.org",
	"soundcloud.com", "sndcdn.com", "medium.com", "tumblr.com",
	"signal.org", "proton.me", "protonvpn.com", "torproject.org",
}

// Matcher, son ek (suffix) eşleştirmesiyle çalışan, eşzamanlı kullanıma
// uygun bir alan adı kümesidir. "discord.com" kuralı "cdn.discord.com" ile de eşleşir.
type Matcher struct {
	mu   sync.RWMutex
	mode Mode
	set  map[string]struct{}
}

// NewMatcher, kip ve alan adlarıyla bir eşleştirici kurar.
func NewMatcher(mode Mode, list []string) *Matcher {
	m := &Matcher{mode: mode}
	m.Set(list)
	return m
}

// Set, listeyi değiştirir.
func (m *Matcher) Set(list []string) {
	set := make(map[string]struct{}, len(list))
	for _, d := range list {
		d = normalize(d)
		if d != "" {
			set[d] = struct{}{}
		}
	}
	m.mu.Lock()
	m.set = set
	m.mu.Unlock()
}

// SetMode, kipi değiştirir.
func (m *Matcher) SetMode(mode Mode) {
	m.mu.Lock()
	m.mode = mode
	m.mu.Unlock()
}

// Mode, geçerli kipi döndürür.
func (m *Matcher) Mode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// List, listedeki alan adlarını döndürür.
func (m *Matcher) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.set))
	for d := range m.set {
		out = append(out, d)
	}
	return out
}

// Apply, verilen ana bilgisayar adına atlatma uygulanıp uygulanmayacağını söyler.
func (m *Matcher) Apply(host string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.mode != ModeList {
		return true
	}
	h := normalize(host)
	for h != "" {
		if _, ok := m.set[h]; ok {
			return true
		}
		i := strings.IndexByte(h, '.')
		if i < 0 {
			break
		}
		h = h[i+1:]
	}
	return false
}

// Parse, her satırda bir alan adı olan metni okur. # ile başlayan satırlar yorumdur.
func Parse(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}

func normalize(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "*.")
	d = strings.TrimSuffix(d, ".")
	return d
}
