// Package adblock, reklam ve izleyici alan adlarına giden bağlantıları
// proxy düzeyinde keser. Hazır listeleri indirir, diske saklar ve günde bir
// kez günceller.
package adblock

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// List, indirilecek bir engelleme listesidir.
type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DefaultLists, varsayılan listelerdir.
var DefaultLists = []List{
	{"adguard-dns", "AdGuard DNS filtresi", "https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt"},
	{"stevenblack", "StevenBlack hosts (reklam + kötü amaçlı)", "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts"},
}

// Blocker, eşzamanlı kullanıma uygun alan adı engelleyicisidir.
type Blocker struct {
	enabled atomic.Bool
	mu      sync.RWMutex
	set     map[string]struct{}
	allow   map[string]struct{}
	updated time.Time

	Blocked atomic.Int64
	// Dir, listelerin saklandığı klasördür.
	Dir   string
	Lists []List
	// Client, listeleri indirmek için kullanılır (DoH destekli olması için
	// motorun çevirici fonksiyonuyla kurulur).
	Client *http.Client
}

// New, boş bir engelleyici kurar.
func New(dir string) *Blocker {
	return &Blocker{Dir: dir, Lists: DefaultLists, set: map[string]struct{}{}, allow: map[string]struct{}{}, Client: http.DefaultClient}
}

func (b *Blocker) SetEnabled(on bool) { b.enabled.Store(on) }
func (b *Blocker) Enabled() bool      { return b.enabled.Load() }

// Count, listedeki alan adı sayısıdır.
func (b *Blocker) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.set)
}

// Updated, listelerin son yüklenme zamanıdır.
func (b *Blocker) Updated() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.updated
}

// SetAllow, engellenmeyecek alan adlarını ayarlar.
func (b *Blocker) SetAllow(list []string) {
	m := map[string]struct{}{}
	for _, d := range list {
		if d = normalize(d); d != "" {
			m[d] = struct{}{}
		}
	}
	b.mu.Lock()
	b.allow = m
	b.mu.Unlock()
}

// Allowed, istisna listesini döndürür.
func (b *Blocker) Allowed() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.allow))
	for d := range b.allow {
		out = append(out, d)
	}
	return out
}

// Block, ana bilgisayar adının ya da üst alan adlarından birinin listede
// olup olmadığını söyler. Engelleyici kapalıysa her zaman false döner.
func (b *Blocker) Block(host string) bool {
	if !b.Enabled() {
		return false
	}
	h := normalize(host)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for d := h; d != ""; {
		if _, ok := b.allow[d]; ok {
			return false
		}
		if _, ok := b.set[d]; ok {
			b.Blocked.Add(1)
			return true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			break
		}
		d = d[i+1:]
	}
	return false
}

// LoadCached, diskteki listeleri yükler. Hiç liste yoksa false döner.
func (b *Blocker) LoadCached() (bool, error) {
	set := map[string]struct{}{}
	var newest time.Time
	found := false
	for _, l := range b.Lists {
		p := filepath.Join(b.Dir, l.ID+".txt")
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		found = true
		Parse(f, set)
		f.Close()
		if st, err := os.Stat(p); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	if !found {
		return false, nil
	}
	b.mu.Lock()
	b.set, b.updated = set, newest
	b.mu.Unlock()
	return true, nil
}

// Update, listeleri indirir, diske yazar ve yeniden yükler.
func (b *Blocker) Update(ctx context.Context) error {
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return err
	}
	var errs []string
	for _, l := range b.Lists {
		if err := b.download(ctx, l); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", l.Name, err))
		}
	}
	if _, err := b.LoadCached(); err != nil {
		return err
	}
	if len(errs) == len(b.Lists) {
		return fmt.Errorf("listeler indirilemedi: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (b *Blocker) download(ctx context.Context, l List) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
	if err != nil {
		return err
	}
	resp, err := b.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := filepath.Join(b.Dir, l.ID+".tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 32<<20)); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, filepath.Join(b.Dir, l.ID+".txt"))
}

// NeedsUpdate, listeler maxAge'den eskiyse true döner.
func (b *Blocker) NeedsUpdate(maxAge time.Duration) bool {
	return time.Since(b.Updated()) > maxAge
}

// Parse, hosts biçimini ("0.0.0.0 alan.adi") ve Adblock biçimini
// ("||alan.adi^") okuyup kümeye ekler. Diğer kurallar yok sayılır.
func Parse(r io.Reader, set map[string]struct{}) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || l[0] == '#' || l[0] == '!' {
			continue
		}
		var d string
		switch {
		case strings.HasPrefix(l, "||"):
			end := strings.IndexAny(l, "^$/")
			if end < 0 || l[end] != '^' || strings.Contains(l[end:], "$") {
				continue // yalnız düz alan adı kuralları
			}
			d = l[2:end]
		case strings.HasPrefix(l, "0.0.0.0 ") || strings.HasPrefix(l, "127.0.0.1 "):
			f := strings.Fields(l)
			if len(f) < 2 {
				continue
			}
			d = f[1]
		default:
			continue
		}
		d = normalize(d)
		if d == "" || d == "localhost" || d == "0.0.0.0" || strings.ContainsAny(d, "*/ ") {
			continue
		}
		set[d] = struct{}{}
	}
}

func normalize(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "*.")
	return strings.TrimSuffix(d, ".")
}
