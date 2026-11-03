// Package control, menü çubuğu uygulamasının motoru yönettiği yerel HTTP
// API'sidir. Yalnız 127.0.0.1'de dinler ve her istekte belirteç ister;
// böylece tarayıcıdaki bir web sayfası motoru yönetemez.
package control

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UmutKDev/Slipstream/internal/adblock"
	"github.com/UmutKDev/Slipstream/internal/config"
	"github.com/UmutKDev/Slipstream/internal/desync"
	"github.com/UmutKDev/Slipstream/internal/dns"
	"github.com/UmutKDev/Slipstream/internal/domains"
	"github.com/UmutKDev/Slipstream/internal/probe"
	"github.com/UmutKDev/Slipstream/internal/profile"
	"github.com/UmutKDev/Slipstream/internal/proxy"
	"github.com/UmutKDev/Slipstream/internal/sysproxy"
)

// Version, motor sürümüdür.
var Version = "0.1.0"

// Server, kontrol API'sidir.
type Server struct {
	Engine *proxy.Engine
	Store  *config.Store
	Token  string
	// ProxyPort, sistem proxy'si açılırken kullanılan porttur.
	ProxyPort int
	Shutdown  func()
	Ads       *adblock.Blocker
	// ProxyHook, sistem proxy'sini networksetup yerine başka bir yolla
	// ayarlar. iOS'ta tünel eklentisi bunu kullanarak proxy ayarını
	// tünelin ağ ayarlarına yazar. nil ise sysproxy paketi kullanılır.
	ProxyHook func(enabled bool, mode sysproxy.Mode) error

	sysMu   sync.Mutex
	sysGen  atomic.Uint64
	sysBusy atomic.Bool

	adUpdating atomic.Bool

	probeMu   sync.Mutex
	probing   bool
	lastProbe *probe.Report
}

// Handler, API yönlendiricisini döndürür.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/profiles", s.profiles)
	mux.HandleFunc("POST /api/profile", s.setProfile)
	mux.HandleFunc("POST /api/strategy", s.setStrategy)
	mux.HandleFunc("POST /api/dns", s.setDNS)
	mux.HandleFunc("POST /api/dns/flush", s.flushDNS)
	mux.HandleFunc("GET /api/domains", s.getDomains)
	mux.HandleFunc("POST /api/domains", s.setDomains)
	mux.HandleFunc("GET /api/logs", s.logs)
	mux.HandleFunc("POST /api/probe", s.runProbe)
	mux.HandleFunc("GET /api/probe", s.getProbe)
	mux.HandleFunc("POST /api/systemproxy", s.setSystemProxy)
	mux.HandleFunc("POST /api/shutdown", s.shutdown)
	mux.HandleFunc("GET /api/learned", s.getLearned)
	mux.HandleFunc("DELETE /api/learned", s.clearLearned)
	mux.HandleFunc("POST /api/autolearn", s.setAutoLearn)
	mux.HandleFunc("GET /api/adblock", s.getAdBlock)
	mux.HandleFunc("POST /api/adblock", s.setAdBlock)
	mux.HandleFunc("POST /api/adblock/update", s.updateAdBlock)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Slipstream-Token")
		if s.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
			http.Error(w, "yetkisiz", http.StatusUnauthorized)
			return
		}
		// DNS yeniden bağlama saldırılarına karşı yalnız yerel Host kabul edilir.
		host := r.Host
		if i := strings.LastIndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "yasak", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	c := s.Store.Get()
	writeJSON(w, map[string]any{
		"version":     Version,
		"listen":      c.Listen,
		"profile":     c.Profile,
		"strategy":    s.Engine.Strategy(),
		"dns":         c.DNS,
		"mode":        c.Mode,
		"systemProxy": c.SystemProxy,
		"proxyMode":   c.ProxyMode,
		"proxyBusy":   s.sysBusy.Load(),
		"stats":       s.Engine.Stats.Snapshot(),
		"autoLearn":   c.AutoLearn,
		"learned":     len(s.Engine.Learned()),
		"adBlock":     s.adInfo(),
	})
}

func (s *Server) profiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"profiles": profile.List(), "dnsProviders": dns.Providers})
}

func (s *Server) setProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p, ok := profile.All[in.ID]
	if !ok {
		http.Error(w, "bilinmeyen profil", 404)
		return
	}
	s.Engine.SetStrategy(p.Strategy)
	c, err := s.Store.Update(func(c *config.Config) { c.Profile = p.ID })
	s.Engine.Log.Infof("profil değişti: %s", p.Name)
	s.respond(w, c, err)
}

func (s *Server) setStrategy(w http.ResponseWriter, r *http.Request) {
	var st desync.Strategy
	if err := readJSON(r, &st); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if st.Name == "" {
		st.Name = "ozel"
	}
	s.Engine.SetStrategy(st)
	c, err := s.Store.Update(func(c *config.Config) { c.Profile = "custom"; c.Custom = &st })
	s.Engine.Log.Infof("özel strateji uygulandı: %s", st.Name)
	s.respond(w, c, err)
}

func (s *Server) setDNS(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Providers []string `json:"providers"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	res, err := dns.New(in.Providers...)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.Engine.SetResolver(res)
	c, err := s.Store.Update(func(c *config.Config) { c.DNS = in.Providers })
	s.Engine.Log.Infof("DNS sağlayıcıları: %s", strings.Join(in.Providers, ", "))
	s.respond(w, c, err)
}

func (s *Server) flushDNS(w http.ResponseWriter, _ *http.Request) {
	s.Engine.Resolver().Flush()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) getDomains(w http.ResponseWriter, _ *http.Request) {
	c := s.Store.Get()
	writeJSON(w, map[string]any{"mode": c.Mode, "domains": c.Domains})
}

func (s *Server) setDomains(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode    domains.Mode `json:"mode"`
		Domains []string     `json:"domains"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if in.Mode != domains.ModeAll && in.Mode != domains.ModeList {
		http.Error(w, "kip all ya da list olmalı", 400)
		return
	}
	s.Engine.Domains.SetMode(in.Mode)
	if in.Domains != nil {
		s.Engine.Domains.Set(in.Domains)
	}
	c, err := s.Store.Update(func(c *config.Config) {
		c.Mode = in.Mode
		if in.Domains != nil {
			c.Domains = in.Domains
		}
	})
	if in.Domains != nil {
		go s.RefreshPAC()
	}
	s.respond(w, c, err)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	writeJSON(w, map[string]any{"entries": s.Engine.Log.Since(since)})
}

// runProbe, otomatik strateji bulmayı arka planda başlatır. "apply" true ise
// en iyi strateji bulununca hemen uygulanır.
func (s *Server) runProbe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Targets []string `json:"targets"`
		Apply   bool     `json:"apply"`
	}
	readJSON(r, &in)
	s.probeMu.Lock()
	if s.probing {
		s.probeMu.Unlock()
		http.Error(w, "deneme zaten sürüyor", http.StatusConflict)
		return
	}
	s.probing = true
	s.probeMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		rep := probe.Run(ctx, s.Engine.Dial, profile.Candidates(), probe.Options{
			Targets:  in.Targets,
			Progress: func(m string) { s.Engine.Log.Infof("[otomatik] %s", m) },
		})
		if in.Apply && rep.Best != nil {
			st := rep.Best.Strategy
			s.Engine.SetStrategy(st)
			s.Store.Update(func(c *config.Config) { c.Profile = "custom"; c.Custom = &st })
			s.Engine.Log.Infof("[otomatik] en iyi strateji uygulandı: %s (%d/%d, %d ms)", st.Name, rep.Best.OK, rep.Best.Total, rep.Best.LatencyMs)
		} else if rep.Best == nil {
			s.Engine.Log.Warnf("[otomatik] çalışan strateji bulunamadı")
		}
		s.probeMu.Lock()
		s.probing, s.lastProbe = false, &rep
		s.probeMu.Unlock()
	}()
	writeJSON(w, map[string]bool{"started": true})
}

func (s *Server) getProbe(w http.ResponseWriter, _ *http.Request) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	writeJSON(w, map[string]any{"running": s.probing, "report": s.lastProbe})
}

func (s *Server) setSystemProxy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool   `json:"enabled"`
		Mode    string `json:"mode"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	mode := s.Store.Get().ProxyMode
	if in.Mode != "" {
		if in.Mode != string(sysproxy.ModeGlobal) && in.Mode != string(sysproxy.ModePAC) {
			http.Error(w, "kip global ya da pac olmalı", 400)
			return
		}
		mode = in.Mode
	}
	c, err := s.Store.Update(func(c *config.Config) { c.SystemProxy = in.Enabled; c.ProxyMode = mode })
	// networksetup her ağ hizmeti için ayrı çalıştığından birkaç saniye
	// sürebilir. Yanıt hemen döner, ayar arka planda sırayla uygulanır.
	go s.applySystemProxy(in.Enabled, sysproxy.Mode(mode))
	s.respond(w, c, err)
}

// applySystemProxy, sistem proxy'sini sırayla değiştirir. Arka arkaya gelen
// isteklerde yalnız sonuncusu uygulanır.
func (s *Server) applySystemProxy(enabled bool, mode sysproxy.Mode) {
	gen := s.sysGen.Add(1)
	s.sysMu.Lock()
	defer s.sysMu.Unlock()
	if s.sysGen.Load() != gen {
		return // daha yeni bir istek geldi
	}
	s.sysBusy.Store(true)
	defer s.sysBusy.Store(false)
	var err error
	if enabled {
		err = s.ApplySystemProxy(mode)
	} else if s.ProxyHook != nil {
		err = s.ProxyHook(false, mode)
	} else {
		err = sysproxy.Disable()
	}
	if err != nil {
		s.Engine.Log.Errorf("sistem proxy'si değiştirilemedi: %v", err)
		return
	}
	if !enabled {
		s.Engine.Log.Infof("sistem proxy'si kapatıldı")
	} else if mode == sysproxy.ModePAC {
		s.Engine.Log.Infof("sistem proxy'si PAC kipinde: yalnız listedeki siteler geçer")
	} else {
		s.Engine.Log.Infof("sistem proxy'si tüm trafik kipinde")
	}
}

// ApplySystemProxy, sistem proxy'sini kipe göre ayarlar. PAC adresine bir
// sürüm parametresi eklenir; macOS PAC dosyasını önbelleğe aldığı için
// liste değişince yeni adres verilmesi gerekir.
func (s *Server) ApplySystemProxy(mode sysproxy.Mode) error {
	if s.ProxyHook != nil {
		return s.ProxyHook(true, mode)
	}
	pac := fmt.Sprintf("http://127.0.0.1:%d/proxy.pac?v=%d", s.ProxyPort, time.Now().Unix())
	return sysproxy.Apply(mode, "127.0.0.1", s.ProxyPort, pac)
}

// RefreshPAC, PAC kipindeyse sistemin PAC dosyasını yeniden okumasını sağlar.
func (s *Server) RefreshPAC() {
	c := s.Store.Get()
	if !c.SystemProxy || c.ProxyMode != string(sysproxy.ModePAC) || (!sysproxy.Supported && s.ProxyHook == nil) {
		return
	}
	s.sysMu.Lock()
	defer s.sysMu.Unlock()
	if err := s.ApplySystemProxy(sysproxy.ModePAC); err != nil {
		s.Engine.Log.Warnf("PAC yenilenemedi: %v", err)
	}
}

func (s *Server) shutdown(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]bool{"ok": true})
	if s.Shutdown != nil {
		go s.Shutdown()
	}
}

func (s *Server) respond(w http.ResponseWriter, c config.Config, err error) {
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, c)
}

func (s *Server) getLearned(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"sites": s.Engine.Learned()})
}

func (s *Server) clearLearned(w http.ResponseWriter, _ *http.Request) {
	s.Engine.SetLearned(nil)
	if s.Engine.OnLearn != nil {
		s.Engine.OnLearn(map[string]desync.Strategy{})
	}
	s.Engine.Log.Infof("öğrenilen site ayarları sıfırlandı")
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) setAutoLearn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.Engine.SetAutoLearn(in.Enabled)
	c, err := s.Store.Update(func(c *config.Config) { c.AutoLearn = in.Enabled })
	s.respond(w, c, err)
}

// AdInfo, reklam engelleyicinin durumudur.
type AdInfo struct {
	Enabled  bool           `json:"enabled"`
	Domains  int            `json:"domains"`
	Blocked  int64          `json:"blocked"`
	Updated  time.Time      `json:"updated"`
	Allow    []string       `json:"allow"`
	Lists    []string       `json:"lists"`
	Catalog  []adblock.List `json:"catalog"`
	Updating bool           `json:"updating"`
}

func (s *Server) adInfo() AdInfo {
	if s.Ads == nil {
		return AdInfo{}
	}
	return AdInfo{
		Enabled: s.Ads.Enabled(), Domains: s.Ads.Count(), Blocked: s.Ads.Blocked.Load(),
		Updated: s.Ads.Updated(), Allow: s.Ads.Allowed(), Lists: s.Ads.ListIDs(),
		Catalog: adblock.Catalog, Updating: s.adUpdating.Load(),
	}
}

func (s *Server) getAdBlock(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.adInfo())
}

func (s *Server) setAdBlock(w http.ResponseWriter, r *http.Request) {
	if s.Ads == nil {
		http.Error(w, "reklam engelleyici yok", 501)
		return
	}
	var in struct {
		Enabled bool     `json:"enabled"`
		Allow   []string `json:"allow"`
		Lists   []string `json:"lists"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.Ads.SetEnabled(in.Enabled)
	if in.Allow != nil {
		s.Ads.SetAllow(in.Allow)
	}
	missing := false
	if in.Lists != nil {
		missing = s.Ads.SetLists(in.Lists)
	}
	_, err := s.Store.Update(func(c *config.Config) {
		c.AdBlock = in.Enabled
		if in.Allow != nil {
			c.AdAllow = in.Allow
		}
		if in.Lists != nil {
			c.AdLists = s.Ads.ListIDs()
		}
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if in.Enabled && (s.Ads.Count() == 0 || missing) {
		go s.runAdUpdate()
	}
	s.Engine.Log.Infof("reklam engelleyici: %v", in.Enabled)
	writeJSON(w, s.adInfo())
}

func (s *Server) updateAdBlock(w http.ResponseWriter, _ *http.Request) {
	if s.Ads == nil {
		http.Error(w, "reklam engelleyici yok", 501)
		return
	}
	go s.runAdUpdate()
	writeJSON(w, map[string]bool{"started": true})
}

func (s *Server) runAdUpdate() {
	if !s.adUpdating.CompareAndSwap(false, true) {
		return // zaten güncelleniyor
	}
	defer s.adUpdating.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.Ads.Update(ctx); err != nil {
		s.Engine.Log.Warnf("reklam listeleri güncellenemedi: %v", err)
		return
	}
	s.Engine.Log.Infof("reklam listeleri güncellendi: %d alan adı", s.Ads.Count())
}

// RunAdUpdates, listeleri gerekirse hemen, sonra düzenli olarak günceller.
// missing, diskte henüz olmayan bir liste seçildiğini bildirir.
func (s *Server) RunAdUpdates(ctx context.Context, missing bool) {
	if s.Ads == nil {
		return
	}
	if missing && s.Ads.Enabled() {
		s.runAdUpdate()
	}
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if s.Ads.Enabled() && s.Ads.NeedsUpdate(24*time.Hour) {
			s.runAdUpdate()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
