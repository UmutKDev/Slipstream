package proxy

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// PACDomains, PAC dosyasına yazılacak alan adlarıdır: kullanıcının listesi
// ve otomatik öğrenmenin bulduğu siteler.
func (e *Engine) PACDomains() []string {
	set := map[string]struct{}{}
	for _, d := range e.Domains.List() {
		set[d] = struct{}{}
	}
	for d := range e.Learned() {
		set[d] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// PAC, yalnız listedeki siteleri bu proxy'ye yollayan bir PAC betiği üretir.
// Proxy'ye ulaşılamazsa tarayıcı DIRECT ile doğrudan bağlanır.
func (e *Engine) PAC(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	target := net.JoinHostPort(host, port)
	var b strings.Builder
	b.WriteString("// Slipstream tarafından üretildi.\nvar domains = [\n")
	for _, d := range e.PACDomains() {
		fmt.Fprintf(&b, "  %s,\n", strconv.Quote(d))
	}
	fmt.Fprintf(&b, `];
function FindProxyForURL(url, host) {
  host = host.toLowerCase();
  for (var i = 0; i < domains.length; i++) {
    var d = domains[i];
    if (host === d || dnsDomainIs(host, "." + d)) {
      return "PROXY %s; SOCKS5 %s; DIRECT";
    }
  }
  return "DIRECT";
}
`, target, target)
	return b.String()
}
