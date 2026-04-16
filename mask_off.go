//go:build linux && !stealth
// +build linux,!stealth

package main

const IsStealth = false

func maskProcess() {
	// No masking
}
