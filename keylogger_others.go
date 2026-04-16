//go:build !windows
// +build !windows

package main

import (
	"context"
)

// StartKeylogger is a stub for non-Windows systems
func StartKeylogger(ctx context.Context, keyChan chan string) {
	// Do nothing on Linux/Mac for now as requested
}
