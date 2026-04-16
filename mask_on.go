//go:build linux && stealth
// +build linux,stealth

package main

import "os"

const IsStealth = true

func maskProcess() {
	// This is a trick to make ps show /proc/self/exe or the link target
	os.Args[0] = "/proc/self/exe"
}
