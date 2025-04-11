package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Global state
var (
	localFileMetadata FileMetadata
	node              host.Host
	syncedPeers       = make(map[string]bool)
	usedEncryption    = false

	// 👇 Peer store
	knownPeers     = make(map[string]peer.AddrInfo)
	knownPeersLock sync.Mutex

	// printLock at the global level
	printLock sync.Mutex
)

func createNode() host.Host {
	node, err := libp2p.New()
	if err != nil {
		log.Fatalf("[INIT][createNode] ❌ Error creating node: %s", err.Error())
	}
	return node
}

func runTargetNode(h host.Host) peer.AddrInfo {
	log.Printf("[Stream][runTargetNode] Registering handlers for Peer ID '%s'", h.ID().String())

	h.SetStreamHandler("/hello/1.0.0", func(s network.Stream) {
		log.Println("[Stream][/hello] 📥 Incoming stream")
		err := readHelloProtocol(s)
		if err != nil {
			log.Printf("[Stream][/hello] ❌ Metadata read failed: %s", err.Error())
			err := s.Reset()
			if err != nil {
				return
			}
		} else {
			err := s.Close()
			if err != nil {
				return
			}
		}
	})
	log.Println("[Stream] Handler registered for /hello/1.0.0")

	h.SetStreamHandler("/file-transfer/1.0.0", func(s network.Stream) {
		log.Printf("[Stream][/file-transfer] 📥 Stream received from %s", s.Conn().RemotePeer())
		handleFileRequest(s)
	})
	log.Println("[Stream] Handler registered for /file-transfer/1.0.0")

	return *host.InfoFromHost(h)
}

func runSourceNode(targetNodeInfo peer.AddrInfo) {
	log.Printf("[CRDT][runSourceNode] 🔁 Syncing to peer %s", targetNodeInfo.ID.String())

	if err := node.Connect(context.Background(), targetNodeInfo); err != nil {
		log.Printf("[CRDT][runSourceNode] ❌ Connect failed: %s", err.Error())
		return
	}

	stream, err := node.NewStream(context.Background(), targetNodeInfo.ID, "/hello/1.0.0")
	if err != nil {
		log.Printf("[CRDT][runSourceNode] ❌ Stream open failed: %s", err.Error())
		return
	}

	log.Println("[CRDT][runSourceNode] 📤 Sending local metadata...")
	if err := SendMetadata(stream, localFileMetadata); err != nil {
		log.Println("[CRDT][runSourceNode] ❌ Send error:", err)
		return
	}

	remoteMeta, err := ReceiveMetadata(stream)
	if err != nil {
		log.Println("[CRDT][runSourceNode] ❌ Receive error:", err)
		return
	}

	localFileMetadata = MergeFileMetadata(localFileMetadata, remoteMeta)
	log.Println("[CRDT][runSourceNode] ✅ Merged metadata from remote:")
	PrintMetadata(localFileMetadata)

	_ = stream.Close()
}

func readHelloProtocol(s network.Stream) error {
	remoteMeta, err := ReceiveMetadata(s)
	if err != nil {
		return err
	}

	peerID := s.Conn().RemotePeer()
	log.Printf("[CRDT][readHelloProtocol] 📥 Received metadata from %s", peerID)
	localFileMetadata = MergeFileMetadata(localFileMetadata, remoteMeta)

	if err := SendMetadata(s, localFileMetadata); err != nil {
		return err
	}

	if !syncedPeers[peerID.String()] {
		syncedPeers[peerID.String()] = true
		go func() {
			log.Printf("[CRDT][readHelloProtocol] 🔁 Syncing back to %s", peerID)
			runSourceNode(peer.AddrInfo{ID: peerID})
		}()
	}

	return nil
}

func main() {
	// Add this in main() near the beginning
	defer func() {
		if r := recover(); r != nil {
			log.Printf("FATAL: Recovered from panic: %v\n", r)
			// Print stack trace
			debug.PrintStack()
			// Wait a bit before exiting to see the logs
			time.Sleep(5 * time.Second)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Encryption CLI flag
	encryptFlag := flag.Bool("E", false, "Enable AES encryption for file transfer")
	flag.Parse()
	usedEncryption = *encryptFlag

	log.Println("[INIT] 🚀 Starting P2P File Sync Node...")

	localFileMetadata = FileMetadata{
		FileName: "example.txt",
		Versions: make(map[string]FileVersion),
		Heads:    []string{},
	}

	hostname, _ := os.Hostname()
	log.Printf("[INIT] 🖥️ Hostname: %s", hostname)

	node = createNode()
	log.Printf("[INIT] 🆔 Peer ID: %s", node.ID().String())

	version := NewFileVersion(node.ID().String(), "initial upload from "+hostname, "CID123456", nil)
	localFileMetadata.AddVersion(version)
	log.Println("[CRDT][main] 📝 Local file version created")

	_ = runTargetNode(node)

	log.Println("[mDNS][main] 🔍 Starting local peer discovery...")
	if err := startMdnsDiscovery(node); err != nil {
		log.Fatalf("[mDNS][main] ❌ Discovery failed: %v", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, node)
	if err != nil {
		log.Fatalf("[PubSub][main] ❌ Init failed: %v", err)
	}
	if err := setupFilePubSub(ctx, ps, node.ID().String()); err != nil {
		log.Fatalf("[PubSub][main] ❌ File announce setup failed: %v", err)
	}

	go func() {
		log.Println("[PubSub] ⏳ Waiting for peer discovery and topic mesh...")
		time.Sleep(15 * time.Second) // Give peers time to join and discover each other
		log.Println("[PubSub] 📢 Sending initial file announcement...")
		announceLocalFiles(node.ID().String())

		// Show available files after initial announcement
		//printLock.Lock()
		//showAvailableFiles()
		//printLock.Unlock()
	}()
	go func() {
		time.Sleep(16 * time.Second)
		reader := bufio.NewReader(os.Stdin)

		for {
			printLock.Lock()
			showAvailableFiles()
			fmt.Println("📁 Enter file name to download, '' to re-announce (leave input empty and press Enter), or press Ctrl+C to exit:")
			fmt.Print("> ")
			printLock.Unlock()

			inputRaw, err := reader.ReadString('\n')
			if err != nil {
				printLock.Lock()
				log.Println("[CLI] ⚠️ Input error:", err)
				printLock.Unlock()
				continue
			}

			input := strings.TrimSpace(inputRaw)

			if input == "" {
				printLock.Lock()
				log.Println("[CLI] 🔁 Manual refresh triggered (empty input).")
				printLock.Unlock()
				announceLocalFiles(node.ID().String())
				continue
			}

			fileRequested := input
			found := false

			knownFilesLock.Lock()
			for peerID, fileList := range knownFiles {
				if peerID == node.ID().String() {
					continue
				}
				for _, file := range fileList {
					if file == fileRequested {
						knownPeersLock.Lock()
						peerInfo, ok := knownPeers[peerID]
						knownPeersLock.Unlock()

						if ok {
							printLock.Lock()
							log.Printf("[CLI] 📥 Requesting file '%s' from peer %s...", fileRequested, peerID)
							printLock.Unlock()

							err := requestFileFromPeer(peerInfo, fileRequested)
							if err != nil {
								printLock.Lock()
								log.Printf("[CLI] ❌ File request failed: %v", err)
								printLock.Unlock()
							}

							// Post-transfer available files view will be handled inside requestFileFromPeer
							found = true
							break
						}
					}
				}
				if found {
					break
				}
			}
			knownFilesLock.Unlock()

			if !found {
				printLock.Lock()
				log.Printf("[CLI] ⚠️ File '%s' not found in known announcements.", fileRequested)
				printLock.Unlock()
			}
		}
	}()

	log.Println("[READY] ✅ Node is up and running. Press Ctrl+C to exit.")
	<-ctx.Done()
}
