// Package profile, Türkiye'deki ISS'ler için hazır atlatma profillerini tutar.
//
// ISS'lerin DPI kuralları zamanla ve bölgeye göre değişir. Bu profiller iyi
// birer başlangıç noktasıdır. Kesin sonuç için "otomatik bul" (probe) modu
// her strateji ailesini kullanıcının kendi hattında dener.
package profile

import (
	"sort"

	"github.com/UmutKDev/slipstream/internal/desync"
)

// Profile, kullanıcıya gösterilen tek bir hazır ayardır.
type Profile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Strategy    desync.Strategy `json:"strategy"`
	DNS         []string        `json:"dns"`
	Order       int             `json:"-"`
}

var httpTricks = desync.Strategy{HostCase: true, HostNoSpace: true, HostMixedValue: true, HTTPSplit: true}

func with(s desync.Strategy, name string) desync.Strategy {
	s.Name = name
	s.HostCase, s.HostNoSpace, s.HostMixedValue, s.HTTPSplit =
		httpTricks.HostCase, httpTricks.HostNoSpace, httpTricks.HostMixedValue, httpTricks.HTTPSplit
	return s
}

// All, tüm hazır profillerdir.
var All = map[string]Profile{
	"dengeli": {
		ID: "dengeli", Name: "Dengeli (önerilen)", Order: 0,
		Description: "TLS kaydını ve TCP paketini SNI'nin ortasından böler. Çoğu hatta hızlı ve yeterli.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitSNI}, "dengeli"),
		DNS:         []string{"cloudflare", "google"},
	},
	"turktelekom": {
		ID: "turktelekom", Name: "Türk Telekom", Order: 1,
		Description: "SNI bölme ve TLS kaydı bölme birlikte. Türk Telekom altyapısını kullanan sanal operatörlerde de dener.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitSNI, DelayMs: 2}, "turktelekom"),
		DNS:         []string{"cloudflare", "quad9"},
	},
	"superonline": {
		ID: "superonline", Name: "Turkcell Superonline", Order: 2,
		Description: "TLS kaydını böler, ilk baytları küçük parçalar halinde yollar.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 8, DelayMs: 1}, "superonline"),
		DNS:         []string{"cloudflare", "google"},
	},
	"vodafone": {
		ID: "vodafone", Name: "Vodafone Net", Order: 3,
		Description: "İlk baytı ayrı gönderir, ardından SNI'den böler ve kısa bekler.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitFixed, SplitPos: 1, DelayMs: 5}, "vodafone"),
		DNS:         []string{"google", "cloudflare"},
	},
	"turknet": {
		ID: "turknet", Name: "TurkNet", Order: 4,
		Description: "Yalnız TCP katmanında SNI'den böler. En az ek yükle çalışır.",
		Strategy:    with(desync.Strategy{Split: desync.SplitSNI, DelayMs: 3}, "turknet"),
		DNS:         []string{"cloudflare", "adguard"},
	},
	"millenicom": {
		ID: "millenicom", Name: "Millenicom ve diğer altyapılar", Order: 5,
		Description: "TLS kaydını böler ve ClientHello'yu 16 baytlık parçalarla gönderir.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 16, DelayMs: 2}, "millenicom"),
		DNS:         []string{"cloudflare", "google"},
	},
	"agresif": {
		ID: "agresif", Name: "Agresif", Order: 6,
		Description: "Her şeyi dener: kayıt bölme, 2 baytlık parçalar ve gecikme. Yavaştır, son çare olarak kullan.",
		Strategy:    with(desync.Strategy{RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 2, DelayMs: 10}, "agresif"),
		DNS:         []string{"cloudflare", "google", "quad9"},
	},
}

// Default, varsayılan profil kimliğidir.
const Default = "dengeli"

// List, profilleri gösterim sırasıyla döndürür.
func List() []Profile {
	out := make([]Profile, 0, len(All))
	for _, p := range All {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Candidates, otomatik bulma modunun denediği stratejilerdir. Hafiften
// ağıra doğru sıralıdır, böylece ilk çalışan genelde en hızlısıdır.
func Candidates() []desync.Strategy {
	c := []desync.Strategy{
		with(desync.Strategy{Split: desync.SplitFixed, SplitPos: 1}, "ilk-bayt"),
		with(desync.Strategy{Split: desync.SplitSNI}, "sni"),
		with(desync.Strategy{RecordSplit: true}, "kayit"),
		with(desync.Strategy{RecordSplit: true, Split: desync.SplitSNI}, "kayit+sni"),
		with(desync.Strategy{RecordSplit: true, Split: desync.SplitSNI, DelayMs: 5}, "kayit+sni+gecikme"),
		with(desync.Strategy{RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 8, DelayMs: 1}, "kayit+parca8"),
		with(desync.Strategy{RecordSplit: true, Split: desync.SplitChunk, ChunkSize: 2, DelayMs: 10}, "agresif"),
	}
	return c
}
