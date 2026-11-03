// Package sysproxy, macOS sistem proxy ayarlarını açıp kapatır.
package sysproxy

// Bypass, proxy dışında tutulan adreslerdir.
var Bypass = []string{"*.local", "169.254/16", "127.0.0.1", "localhost", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// Mode, sistem proxy'sinin nasıl ayarlanacağıdır.
type Mode string

const (
	// ModeGlobal, tüm web trafiğini proxy'den geçirir.
	ModeGlobal Mode = "global"
	// ModePAC, yalnız listedeki siteleri proxy'den geçirir.
	ModePAC Mode = "pac"
)

// Apply, kipe göre sistem proxy'sini açar.
func Apply(mode Mode, host string, port int, pacURL string) error {
	if mode == ModePAC {
		return EnablePAC(pacURL)
	}
	return Enable(host, port)
}
