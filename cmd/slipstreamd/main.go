// slipstreamd, Slipstream'in DPI atlatma motorudur. Menü çubuğu uygulaması bu
// programı arka planda çalıştırır; tek başına komut satırından da kullanılabilir.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/UmutKDev/Slipstream/internal/app"
	"github.com/UmutKDev/Slipstream/internal/config"
	"github.com/UmutKDev/Slipstream/internal/control"
	"github.com/UmutKDev/Slipstream/internal/dns"
	"github.com/UmutKDev/Slipstream/internal/domains"
	"github.com/UmutKDev/Slipstream/internal/probe"
	"github.com/UmutKDev/Slipstream/internal/profile"
	"github.com/UmutKDev/Slipstream/internal/proxy"
	"github.com/UmutKDev/Slipstream/internal/sysproxy"
)

const usage = `Slipstream — macOS için DPI atlatma motoru

Kullanım:
  slipstreamd run      [bayraklar]   Proxy'yi başlatır (varsayılan komut)
  slipstreamd probe    [bayraklar]   Hattında çalışan stratejiyi bulur
  slipstreamd profiles               Hazır ISS profillerini listeler
  slipstreamd sysproxy on|off        Sistem proxy'sini elle açar ya da kapatır
  slipstreamd version

"slipstreamd <komut> -h" ile o komutun bayraklarını görebilirsin.
`

func main() {
	log.SetFlags(log.Ltime)
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "run":
		err = run(args)
	case "probe":
		err = runProbe(args)
	case "profiles":
		for _, p := range profile.List() {
			fmt.Printf("%-12s %s\n             %s\n", p.ID, p.Name, p.Description)
		}
	case "sysproxy":
		err = sysproxyCmd(args)
	case "version":
		fmt.Println("slipstreamd", control.Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hata:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "ayar dosyası")
	listen := fs.String("listen", "", "proxy adresi (ayar dosyasındakini ezer)")
	api := fs.String("api", "127.0.0.1:8882", "kontrol API adresi")
	token := fs.String("token", os.Getenv("SLIPSTREAM_TOKEN"), "kontrol API belirteci (boşsa rastgele üretilir)")
	prof := fs.String("profile", "", "başlangıç profili")
	sys := fs.Bool("system-proxy", false, "açılışta sistem proxy'sini ayarla, kapanışta geri al")
	fs.Parse(args)

	store, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("ayarlar okunamadı: %w", err)
	}
	cfg, err := store.Update(func(c *config.Config) {
		if *listen != "" {
			c.Listen = *listen
		}
		if *prof != "" {
			c.Profile = *prof
		}
	})
	if err != nil {
		log.Printf("uyarı: ayarlar yazılamadı: %v", err)
	}

	rt, err := app.Build(store)
	if err != nil {
		return err
	}
	eng, ads, adMissing := rt.Engine, rt.Ads, rt.AdMissing

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("proxy portu açılamadı: %w", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	if *token == "" {
		b := make([]byte, 16)
		rand.Read(b)
		*token = hex.EncodeToString(b)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctl := &control.Server{Engine: eng, Store: store, Token: *token, ProxyPort: port, Shutdown: stop, Ads: ads}
	go ctl.RunAdUpdates(ctx, adMissing)
	// PAC kipinde yeni öğrenilen site PAC dosyasına girer; sistemin
	// dosyayı yeniden okuması için adres yenilenir.
	rt.OnLearned = func() { go ctl.RefreshPAC() }
	apiLn, err := net.Listen("tcp", *api)
	if err != nil {
		return fmt.Errorf("kontrol API portu açılamadı: %w", err)
	}
	apiSrv := &http.Server{Handler: ctl.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go apiSrv.Serve(apiLn)

	useSys := *sys || cfg.SystemProxy
	if useSys && sysproxy.Supported {
		if *sys && !cfg.SystemProxy {
			store.Update(func(c *config.Config) { c.SystemProxy = true })
		}
		mode := sysproxy.Mode(cfg.ProxyMode)
		if err := ctl.ApplySystemProxy(mode); err != nil {
			eng.Log.Errorf("sistem proxy'si ayarlanamadı: %v", err)
		} else if mode == sysproxy.ModePAC {
			eng.Log.Infof("sistem proxy'si PAC kipinde: yalnız listedeki siteler 127.0.0.1:%d üzerinden geçer", port)
		} else {
			eng.Log.Infof("sistem proxy'si 127.0.0.1:%d olarak ayarlandı", port)
		}
	}

	// Menü çubuğu uygulaması bu satırı okuyarak motorun hazır olduğunu anlar.
	ready, _ := json.Marshal(map[string]any{"event": "ready", "proxy": ln.Addr().String(), "api": apiLn.Addr().String(), "version": control.Version})
	fmt.Println(string(ready))
	eng.Log.Infof("Slipstream %s başladı, profil: %s, DNS: %s", control.Version, cfg.Profile, strings.Join(cfg.DNS, ", "))

	err = eng.Serve(ctx, ln)

	// Kapanışta sistem proxy'sini mutlaka geri al; yoksa internet kesilir.
	if store.Get().SystemProxy || *sys {
		if err := sysproxy.Disable(); err != nil {
			log.Printf("sistem proxy'si kapatılamadı: %v", err)
		}
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	apiSrv.Shutdown(shCtx)
	return err
}

func runProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	targets := fs.String("targets", strings.Join(probe.DefaultTargets, ","), "virgülle ayrılmış alan adları")
	dnsList := fs.String("dns", "cloudflare,google", "DoH sağlayıcıları")
	asJSON := fs.Bool("json", false, "sonucu JSON olarak yaz")
	fs.Parse(args)

	res, err := dns.New(strings.Split(*dnsList, ",")...)
	if err != nil {
		return err
	}
	eng := proxy.NewEngine(profile.All[profile.Default].Strategy, res, domains.NewMatcher(domains.ModeAll, nil))
	rep := probe.Run(context.Background(), eng.Dial, profile.Candidates(), probe.Options{
		Targets: strings.Split(*targets, ","),
		Progress: func(m string) {
			if !*asJSON {
				fmt.Println("•", m)
			}
		},
	})
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(rep)
	}
	fmt.Println()
	if len(rep.Blocked) > 0 {
		fmt.Println("Engelli görünen siteler:", strings.Join(rep.Blocked, ", "))
	}
	if rep.Best == nil {
		fmt.Println("Çalışan bir strateji bulunamadı.")
		return nil
	}
	b := rep.Best
	fmt.Printf("En iyi strateji: %s (%d/%d başarılı, ortalama %d ms)\n", b.Strategy.Name, b.OK, b.Total, b.LatencyMs)
	return nil
}

func sysproxyCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("on ya da off yaz")
	}
	switch args[0] {
	case "on":
		port := 8881
		if len(args) > 1 {
			port, _ = strconv.Atoi(args[1])
		}
		return sysproxy.Enable("127.0.0.1", port)
	case "off":
		return sysproxy.Disable()
	}
	return fmt.Errorf("bilinmeyen değer %q", args[0])
}
