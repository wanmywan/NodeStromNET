//go:build !controller && !linux && !windows
// +build !controller,!linux,!windows

package main

func daemonize(runFunc func()) {
	// No-op for non-Linux systems, just run
	runFunc()
}
