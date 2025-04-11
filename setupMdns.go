package main

import (
	"context"
	"log"

	"strings"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// Service tag used for peer discovery
const mdnsServiceTag = "p2p-office-mdns-sync"

// Shared map for storing full AddrInfo of discovered peers
//var (
//	knownPeers     = make(map[string]peer.AddrInfo)
//	knownPeersLock sync.Mutex
//) // declared in main.go

type mdnsNotifee struct {
	h host.Host
}

// Called when a peer is discovered via mDNS
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	log.Printf("[mDNS][HandlePeerFound] 📡 Found peer ID: %s", pi.ID.String())
	log.Printf("[mDNS][HandlePeerFound] 🔌 Addrs: %v", pi.Addrs)

	// Try connecting to peer
	if err := n.h.Connect(context.Background(), pi); err != nil {
		log.Printf("[mDNS][HandlePeerFound] ❌ Failed to connect to %s: %v", pi.ID.String(), err)
		return
	}

	// Try extracting IP
	var peerIP string
	for _, addr := range pi.Addrs {
		if ip := extractIP(addr.String()); ip != "" {
			peerIP = ip
			break
		}
	}

	log.Printf("[mDNS][HandlePeerFound] ✅ Connected to peer %s at IP %s", pi.ID.String(), peerIP)

	// 🔐 Store AddrInfo for file transfers
	knownPeersLock.Lock()
	knownPeers[pi.ID.String()] = pi
	knownPeersLock.Unlock()
	log.Printf("[mDNS][HandlePeerFound] 🗂️  Stored AddrInfo for peer %s", pi.ID.String())

	// 🔁 Start CRDT metadata sync
	go runSourceNode(pi)
}

// Starts mDNS discovery service
func startMdnsDiscovery(h host.Host) error {
	log.Printf("[mDNS][startMdnsDiscovery] 🚀 Starting mDNS with tag '%s'", mdnsServiceTag)

	service := mdns.NewMdnsService(h, mdnsServiceTag, &mdnsNotifee{h: h})
	err := service.Start()
	if err != nil {
		log.Printf("[mDNS][startMdnsDiscovery] ❌ Failed to start mDNS: %v", err)
	} else {
		log.Printf("[mDNS][startMdnsDiscovery] ✅ mDNS service started successfully")
	}
	return err
}

// Extracts IP address from multiaddr string
func extractIP(addr string) string {
	if strings.HasPrefix(addr, "/ip4/") || strings.HasPrefix(addr, "/ip6/") {
		parts := strings.Split(addr, "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "ip4" || parts[i] == "ip6" {
				return parts[i+1]
			}
		}
	}
	return ""
}
