// Package sysproxy, macOS sistem proxy ayarlarını açıp kapatır.
package sysproxy

// Bypass, proxy dışında tutulan adreslerdir.
var Bypass = []string{"*.local", "169.254/16", "127.0.0.1", "localhost", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
