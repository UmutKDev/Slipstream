// Package app, motoru ayarlardan kurar. macOS'taki slipstreamd ile iOS tünel
// eklentisi aynı kurulumu kullanır.
package app

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/UmutKDev/Slipstream/internal/adblock"
	"github.com/UmutKDev/Slipstream/internal/config"
	"github.com/UmutKDev/Slipstream/internal/desync"
	"github.com/UmutKDev/Slipstream/internal/dns"
	"github.com/UmutKDev/Slipstream/internal/domains"
	"github.com/UmutKDev/Slipstream/internal/profile"
	"github.com/UmutKDev/Slipstream/internal/proxy"
)

// Runtime, kurulmuş motor ve yardımcılarıdır.
type Runtime struct {
	Store  *config.Store
	Engine *proxy.Engine
	Ads    *adblock.Blocker
	// AdMissing, seçili listelerden diskte olmayan varsa true'dur.
	AdMissing bool
	// OnLearned, yeni bir site stratejisi öğrenilip diske yazılınca çağrılır.
	OnLearned func()
}

// Build, ayarlara göre motoru ve reklam engelleyiciyi kurar.
func Build(store *config.Store) (*Runtime, error) {
	cfg := store.Get()
	res, err := dns.New(cfg.DNS...)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{Store: store}
	eng := proxy.NewEngine(cfg.Strategy(), res, domains.NewMatcher(cfg.Mode, cfg.Domains))
	eng.Candidates = profile.Candidates()
	eng.SetAutoLearn(cfg.AutoLearn)
	dataDir := store.Dir()
	eng.SetLearned(config.LoadLearned(dataDir))
	eng.OnLearn = func(m map[string]desync.Strategy) {
		if err := config.SaveLearned(dataDir, m); err != nil {
			eng.Log.Warnf("öğrenilen ayarlar yazılamadı: %v", err)
		}
		if rt.OnLearned != nil {
			rt.OnLearned()
		}
	}

	ads := adblock.New(filepath.Join(dataDir, "adblock"))
	ads.SetAllow(cfg.AdAllow)
	ads.SetEnabled(cfg.AdBlock)
	rt.AdMissing = ads.SetLists(cfg.AdLists)
	// Listeler de engelli olabilir; motorun DoH çözücüsüyle indirilir.
	ads.Client = &http.Client{Timeout: time.Minute, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			h, p, _ := net.SplitHostPort(addr)
			n, _ := strconv.Atoi(p)
			return eng.Dial(ctx, h, n)
		},
	}}
	if ads.Count() > 0 {
		eng.Log.Infof("reklam engelleyici: %d alan adı yüklendi", ads.Count())
	}
	eng.Ads = ads
	rt.Engine, rt.Ads = eng, ads
	return rt, nil
}
