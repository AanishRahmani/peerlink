package main

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"sync"
)

// Global state declaration
var (
	localFileMetadata FileMetadata
	node              host.Host
	syncedPeers       = make(map[string]bool)
	usedEncryption    = false

	// 👇 Peer store
	knownPeers     = make(map[string]peer.AddrInfo)
	knownPeersLock sync.Mutex

	// printLock at the global level
	printLock       sync.Mutex
	fileMetadataMap = make(map[string]FileMetadata) // key: file name

)
