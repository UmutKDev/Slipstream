//go:build !darwin || ios

package sysproxy

import "errors"

var errUnsupported = errors.New("sysproxy: bu platformda sistem proxy'si yönetilemiyor")

func Enable(string, int) error { return errUnsupported }
func Disable() error           { return errUnsupported }
func EnablePAC(string) error   { return errUnsupported }

// Supported, bu platformda sistem proxy'sinin yönetilebildiğini söyler.
const Supported = false
