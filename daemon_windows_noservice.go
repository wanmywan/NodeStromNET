//go:build windows && !controller && noservice
// +build windows,!controller,noservice

package main

// daemonize for 'noservice' build just runs the function directly
// It does NOT install a service or hide the window (unless built with -H=windowsgui)
func daemonize(runFunc func()) {
	runFunc()
}
