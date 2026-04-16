//go:build !controller && !linux
// +build !controller,!linux

package main

import (
	"github.com/libp2p/go-libp2p/core/network"
)

func handleMemExecStream(s network.Stream) {
	defer s.Close()
	s.Write([]byte("Error: MemExec only supports Linux\n"))
}
