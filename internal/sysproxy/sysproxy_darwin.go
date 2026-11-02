//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Services, etkin ağ hizmetlerini (Wi-Fi, Ethernet vb.) listeler.
func Services() ([]string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	var s []string
	for i, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		// İlk satır açıklamadır; "*" ile başlayanlar devre dışıdır.
		if i == 0 || l == "" || strings.HasPrefix(l, "*") {
			continue
		}
		s = append(s, l)
	}
	return s, nil
}

func run(args ...string) error {
	out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("networksetup %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// Enable, tüm etkin hizmetlerde HTTP ve HTTPS proxy'sini host:port'a ayarlar.
func Enable(host string, port int) error {
	svcs, err := Services()
	if err != nil {
		return err
	}
	p := strconv.Itoa(port)
	for _, s := range svcs {
		for _, cmd := range [][]string{
			{"-setwebproxy", s, host, p},
			{"-setsecurewebproxy", s, host, p},
			{"-setproxybypassdomains", s},
		} {
			if cmd[0] == "-setproxybypassdomains" {
				cmd = append(cmd, Bypass...)
			}
			if err := run(cmd...); err != nil {
				return err
			}
		}
	}
	return nil
}

// Disable, HTTP ve HTTPS proxy'lerini kapatır.
func Disable() error {
	svcs, err := Services()
	if err != nil {
		return err
	}
	for _, s := range svcs {
		if err := run("-setwebproxystate", s, "off"); err != nil {
			return err
		}
		if err := run("-setsecurewebproxystate", s, "off"); err != nil {
			return err
		}
	}
	return nil
}

// Supported, bu platformda sistem proxy'sinin yönetilebildiğini söyler.
const Supported = true
