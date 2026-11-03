// Package slipcore, Slipstream motorunun iOS'a açılan yüzüdür. gomobile bind
// ile Slipcore.xcframework olarak derlenir ve Packet Tunnel eklentisi
// tarafından kullanılır.
//
// Akış: eklenti Start ile motoru başlatır, dönen proxy ayarlarını tünelin
// ağ ayarlarına yazar. Tünele yalnız sahte DNS adresi yönlendirilir; oraya
// gelen paketler HandlePacket ile buraya verilir, yanıtlar Delegate'in
// WritePacket yöntemiyle geri yazılır. Uygulama motoru Request ile yönetir
// (macOS'taki kontrol API'sinin aynısı, yalnız ağ yerine bellek içinden).
package slipcore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UmutKDev/Slipstream/internal/app"
	"github.com/UmutKDev/Slipstream/internal/config"
	"github.com/UmutKDev/Slipstream/internal/control"
	"github.com/UmutKDev/Slipstream/internal/profile"
	"github.com/UmutKDev/Slipstream/internal/sysproxy"
	"github.com/UmutKDev/Slipstream/internal/tunnel"
)

// Delegate, eklentinin Swift tarafında uygulanır.
type Delegate interface {
	// WritePacket, tünele yazılacak bir IP paketidir (DNS yanıtı).
	WritePacket(pkt []byte)
	// ProxyChanged, proxy ayarı değişince yeni ayarları JSON olarak verir.
	// Biçim Start'ın dönüş değeriyle aynıdır.
	ProxyChanged(settings string)
}

// ProxySettings, tünelin proxy ayarıdır.
type proxySettings struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // "global" ya da "pac"
	Host    string `json:"host"`
	Port    int    `json:"port"`
	PAC     string `json:"pac,omitempty"`
}

type options struct {
	Profile string `json:"profile"`
}

type instance struct {
	rt       *app.Runtime
	ctl      *control.Server
	handler  http.Handler
	dns      *tunnel.DNS
	delegate Delegate
	port     int
	cancel   context.CancelFunc
	done     chan struct{}
	sem      chan struct{}
}

var (
	mu  sync.Mutex
	cur *instance
)

// Bellek sınırı: Packet Tunnel eklentisi yaklaşık 50 MB'ı aşınca iOS onu
// kapatır. Go'nun yığını bunun altında tutulur.
const memoryLimit = 28 << 20

// Start, motoru başlatır. dataDir ayarların ve listelerin tutulduğu
// klasördür (uygulama grubunun ortak klasörü). optionsJSON boş olabilir.
// Dönen değer proxy ayarlarının JSON'udur.
func Start(dataDir string, optionsJSON string, d Delegate) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if cur != nil {
		return cur.settingsJSON(), nil
	}
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(memoryLimit)

	var opt options
	if strings.TrimSpace(optionsJSON) != "" {
		if err := json.Unmarshal([]byte(optionsJSON), &opt); err != nil {
			return "", fmt.Errorf("seçenekler okunamadı: %w", err)
		}
	}
	path := filepath.Join(dataDir, "config.json")
	_, statErr := os.Stat(path)
	fresh := errors.Is(statErr, fs.ErrNotExist)
	store, err := config.Load(path)
	if err != nil {
		return "", fmt.Errorf("ayarlar okunamadı: %w", err)
	}
	store.Update(func(c *config.Config) {
		if fresh {
			// iOS'ta proxy tünelin bir parçası; varsayılan olarak açık.
			c.SystemProxy = true
			c.ProxyMode = string(sysproxy.ModeGlobal)
		}
		if opt.Profile != "" {
			if _, ok := profile.All[opt.Profile]; ok || opt.Profile == "custom" {
				c.Profile = opt.Profile
				if p, ok := profile.All[opt.Profile]; ok && len(p.DNS) > 0 {
					c.DNS = p.DNS
				}
			}
		}
	})
	cfg := store.Get()

	rt, err := app.Build(store)
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("proxy portu açılamadı: %w", err)
		}
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	tok := make([]byte, 16)
	rand.Read(tok)
	ctx, cancel := context.WithCancel(context.Background())
	in := &instance{
		rt: rt, delegate: d, port: port, cancel: cancel,
		done: make(chan struct{}), sem: make(chan struct{}, 64),
		dns: &tunnel.DNS{Resolver: rt.Engine.Resolver, Ads: rt.Ads},
	}
	in.ctl = &control.Server{
		Engine: rt.Engine, Store: store, Token: hex.EncodeToString(tok), ProxyPort: port,
		Ads: rt.Ads, Shutdown: func() { go Stop() },
		ProxyHook: func(bool, sysproxy.Mode) error {
			// Ayar zaten Store'a yazıldı; güncel hâli Swift'e bildirilir.
			if in.delegate != nil {
				in.delegate.ProxyChanged(in.settingsJSON())
			}
			return nil
		},
	}
	in.handler = in.ctl.Handler()
	rt.OnLearned = func() { go in.ctl.RefreshPAC() }

	go in.ctl.RunAdUpdates(ctx, rt.AdMissing)
	go func() {
		defer close(in.done)
		rt.Engine.Serve(ctx, ln)
	}()
	go freeMemory(ctx)

	rt.Engine.Log.Infof("Slipstream %s iOS'ta başladı, profil: %s, DNS: %s", control.Version, cfg.Profile, strings.Join(cfg.DNS, ", "))
	cur = in
	return in.settingsJSON(), nil
}

func (in *instance) settingsJSON() string {
	c := in.rt.Store.Get()
	s := proxySettings{Enabled: c.SystemProxy, Mode: c.ProxyMode, Host: "127.0.0.1", Port: in.port}
	if s.Mode == "" {
		s.Mode = string(sysproxy.ModeGlobal)
	}
	if s.Mode == string(sysproxy.ModePAC) {
		s.PAC = in.rt.Engine.PAC(net.JoinHostPort(s.Host, strconv.Itoa(in.port)))
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// freeMemory, büyük liste yüklemelerinden sonra boş belleği sisteme geri
// verir; iOS eklentinin yerleşik belleğine bakar, Go'nun yığın sınırına değil.
func freeMemory(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			debug.FreeOSMemory()
		}
	}
}

// Stop, motoru durdurur. Çalışmıyorsa bir şey yapmaz.
func Stop() {
	mu.Lock()
	in := cur
	cur = nil
	mu.Unlock()
	if in == nil {
		return
	}
	in.cancel()
	select {
	case <-in.done:
	case <-time.After(3 * time.Second):
	}
}

// Running, motorun çalışıp çalışmadığını söyler.
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return cur != nil
}

// HandlePacket, tünelden okunan bir IP paketini işler. Yalnız 53. porta
// gelen IPv4 UDP paketleri (DNS) yanıtlanır, gerisi atılır. Çağrı hemen
// döner; yanıt Delegate.WritePacket ile gelir.
func HandlePacket(pkt []byte) {
	mu.Lock()
	in := cur
	mu.Unlock()
	if in == nil {
		return
	}
	// gomobile dilimi kopyalamadan verir; arka planda kullanmak için kopyala.
	p, err := tunnel.ParseUDP4(append([]byte(nil), pkt...))
	if err != nil || p.DstPort != 53 {
		return
	}
	select {
	case in.sem <- struct{}{}:
	default:
		return // çok fazla bekleyen sorgu; istemci yeniden dener
	}
	go func() {
		defer func() { <-in.sem }()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		resp, err := in.dns.Answer(ctx, p.Payload)
		if err != nil {
			return
		}
		if in.delegate != nil {
			in.delegate.WritePacket(p.Reply(resp))
		}
	}()
}

type response struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// Request, kontrol API'sine bellek içinden istek gönderir. method ve path
// macOS'taki HTTP API'siyle aynıdır (ör. "GET", "/api/status"). Dönen JSON
// {"status": 200, "body": "..."} biçimindedir.
func Request(method, path, body string) string {
	mu.Lock()
	in := cur
	mu.Unlock()
	if in == nil {
		b, _ := json.Marshal(response{Status: 503, Body: "motor çalışmıyor"})
		return string(b)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	r.Header.Set("X-Slipstream-Token", in.ctl.Token)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	in.handler.ServeHTTP(w, r)
	b, _ := json.Marshal(response{Status: w.Code, Body: w.Body.String()})
	return string(b)
}

// Profiles, hazır ISS profillerini JSON olarak döndürür; motor kapalıyken
// uygulamanın profil listesini göstermesi için.
func Profiles() string {
	b, _ := json.Marshal(map[string]any{"profiles": profile.List()})
	return string(b)
}

// Version, motor sürümüdür.
func Version() string { return control.Version }

// MemoryInUse, Go çalışma zamanının işletim sisteminden aldığı belleği
// bayt olarak döndürür (tanılama için).
func MemoryInUse() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Sys - m.HeapReleased)
}
