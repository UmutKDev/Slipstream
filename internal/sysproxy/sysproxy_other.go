//go:build !darwin

package sysproxy

import "errors"

var errUnsupported = errors.New("sysproxy: yalnız macOS destekleniyor")

func Enable(string, int) error { return errUnsupported }
func Disable() error           { return errUnsupported }

// Supported, bu platformda sistem proxy'sinin yönetilebildiğini söyler.
const Supported = false
