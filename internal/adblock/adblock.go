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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Default     bool   `json:"default"`
	// Builtin, indirilmeyen, uygulamayla gelen alan adlarıdır.
	Builtin []string `json:"-"`
}

// Catalog, seçilebilecek bütün listelerdir. Hepsi DNS düzeyinde, yani alan
// adı bazında çalışır; sayfa içi (kozmetik) kurallar içermez.
var Catalog = []List{
	{ID: "video-ads", Name: "Video reklam kalkanı",
		Description: "Oynatıcılardaki ön reklamları (VAST/IMA) sunan alan adları. Reklam yüklenemeyince oynatıcı doğrudan filme geçer.",
		Default:     true, Builtin: videoAdDomains},
	{"hagezi-pro", "HaGeZi Pro",
		"Reklam, izleyici, video reklam sunucuları (VAST) ve analitik. GitHub'daki en kapsamlı DNS listelerinden biri.",
		"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/pro-onlydomains.txt", true, nil},
	{"hagezi-popup", "HaGeZi Açılır Pencere Reklamları",
		"Film ve dizi sitelerindeki tıklayınca açılan sekme, yönlendirme ve popunder reklam ağları.",
		"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/popupads-onlydomains.txt", true, nil},
	{"turk-adlist", "Türk reklam listesi",
		"Türk sitelerine özgü reklam ve izleyici alan adları (bkrucarci/turk-adlist).",
		"https://raw.githubusercontent.com/bkrucarci/turk-adlist/master/hosts", true, nil},
	{"adguard-dns", "AdGuard DNS filtresi",
		"AdGuard'ın genel reklam ve izleyici listesi.",
		"https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt", true, nil},
	{"stevenblack", "StevenBlack hosts",
		"Reklam ve kötü amaçlı alan adları. HaGeZi Pro ile büyük ölçüde örtüşür.",
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts", false, nil},
	{"hagezi-tif", "HaGeZi Tehdit İstihbaratı (mini)",
		"Kimlik avı, dolandırıcılık ve kötü amaçlı yazılım alan adları.",
		"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/tif.mini-onlydomains.txt", false, nil},
}

// videoAdDomains, oynatıcı içi reklam sunucularıdır. Genel listeler
// Google IMA SDK'yı bazı sitelerde bozulma olmasın diye engellemez; film
// sitelerindeki "reklamı geç" ekranının çoğu buradan gelir.
var videoAdDomains = []string{
	"imasdk.googleapis.com",
	"s0.2mdn.net",
	"ad.doubleclick.net",
	"pubads.g.doubleclick.net",
	"securepubads.g.doubleclick.net",
	"googleads.g.doubleclick.net",
	"vid.springserve.com",
	"ads.stickyadstv.com",
	"ad.lkqd.net",
	"v.lkqd.net",
	"ssp.lkqd.net",
	"vast.adsafeprotected.com",
	"ads.adaptv.advertising.com",
	"vast.yashi.com",
	"ib.adnxs.com",
	"aax.amazon-adsystem.com",
	"cdn.vidoomy.com",
	"ads.vidoomy.com",
	"a.vdo.ai",
	"tag.aniview.com",
	"player.aniview.com",
	"cdn.aniview.com",
	"ads.viralize.tv",
	"vpaid.pubmatic.com",
	"ads.pubmatic.com",
	"rtb.adx1.com",
	"hb.improvedigital.com",
}

// DefaultLists, varsayılan olarak açık listelerdir.
var DefaultLists = Lookup(nil)

// DefaultIDs, varsayılan listelerin kimlikleridir.
func DefaultIDs() []string {
	var ids []string
	for _, l := range Catalog {
		if l.Default {
			ids = append(ids, l.ID)
		}
	}
	return ids
}

// Lookup, kimliklere karşılık gelen listeleri katalog sırasıyla döndürür.
// ids boşsa varsayılan listeler döner.
func Lookup(ids []string) []List {
	if len(ids) == 0 {
		ids = DefaultIDs()
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []List
	for _, l := range Catalog {
		if want[l.ID] {
			out = append(out, l)
		}
	}
	return out
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

// SetLists, kullanılacak listeleri değiştirir. Diskte olmayan liste varsa
// true döner; çağıran Update ile indirmelidir.
func (b *Blocker) SetLists(ids []string) (missing bool) {
	lists := Lookup(ids)
	b.mu.Lock()
	b.Lists = lists
	b.mu.Unlock()
	for _, l := range lists {
		if l.URL == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(b.Dir, l.ID+".txt")); err != nil {
			missing = true
		}
	}
	b.LoadCached()
	return missing
}

// ListIDs, kullanılan listelerin kimlikleridir.
func (b *Blocker) ListIDs() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ids := make([]string, len(b.Lists))
	for i, l := range b.Lists {
		ids[i] = l.ID
	}
	return ids
}

func (b *Blocker) lists() []List {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]List(nil), b.Lists...)
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
	for _, l := range b.lists() {
		if l.Builtin != nil {
			for _, d := range l.Builtin {
				set[d] = struct{}{}
			}
			continue
		}
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
		b.mu.Lock()
		b.set = set
		b.mu.Unlock()
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
	lists := b.lists()
	downloadable := 0
	for _, l := range lists {
		if l.URL == "" {
			continue
		}
		downloadable++
		if err := b.download(ctx, l); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", l.Name, err))
		}
	}
	if _, err := b.LoadCached(); err != nil {
		return err
	}
	if len(errs) > 0 && len(errs) == downloadable {
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

// Parse, hosts biçimini ("0.0.0.0 alan.adi"), Adblock biçimini
// ("||alan.adi^") ve satır başına bir alan adı biçimini okuyup kümeye ekler. Diğer kurallar yok sayılır.
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
		case !strings.ContainsAny(l, " \t") && strings.Contains(l, "."):
			// Yalnız alan adı içeren satır (HaGeZi "onlydomains" biçimi).
			d = l
		default:
			continue
		}
		d = normalize(d)
		if d == "" || d == "localhost" || d == "0.0.0.0" || strings.ContainsAny(d, "*/ |^$@#!:") {
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
