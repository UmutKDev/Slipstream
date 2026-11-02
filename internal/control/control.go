// Package control, menü çubuğu uygulamasının motoru yönettiği yerel HTTP
// API'sidir. Yalnız 127.0.0.1'de dinler ve her istekte belirteç ister;
// böylece tarayıcıdaki bir web sayfası motoru yönetemez.
package control

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UmutKDev/gecit/internal/config"
	"github.com/UmutKDev/gecit/internal/desync"
	"github.com/UmutKDev/gecit/internal/dns"
	"github.com/UmutKDev/gecit/internal/domains"
	"github.com/UmutKDev/gecit/internal/probe"
	"github.com/UmutKDev/gecit/internal/profile"
	"github.com/UmutKDev/gecit/internal/proxy"
	"github.com/UmutKDev/gecit/internal/sysproxy"
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
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Gecit-Token")
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
		"stats":       s.Engine.Stats.Snapshot(),
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
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var err error
	if in.Enabled {
		err = sysproxy.Enable("127.0.0.1", s.ProxyPort)
	} else {
		err = sysproxy.Disable()
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	c, err := s.Store.Update(func(c *config.Config) { c.SystemProxy = in.Enabled })
	s.Engine.Log.Infof("sistem proxy'si: %v", in.Enabled)
	s.respond(w, c, err)
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
