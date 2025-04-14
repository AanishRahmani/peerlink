package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime/debug"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

/*
1 create node
	- initializes and returns a new Libp2p host.
	- calls libp2p.New() to create a P2P node.


3 run target node [DONE]
	- registers stream handlers on your node.
	- registers /hello/1.0.0 → CRDT Metadata sync.
	- registers /file-transfer/1.0.0 → File download.
	- Returns peer address info for advertisement.

4 run source node [DONE]
	- initiates metadata sync with a specific peer.
	- connects to the target peer.
	- opens a /hello/1.0.0 stream.
	- sends your local file metadata map.
	- receives peer’s metadata map.
	- merges remote and local metadata using MergeFileMetadata.
	- Saves merged metadata to disk (sync-metadata.json).

5 read hello protocol [DONE]
	- handle metadata received from a peer when they initiate sync.
	- receives the peer’s metadata map.
	- merges it into your local map.
	- sends your metadata map back.


*/

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

func runSourceNode(targetNodeInfo peer.AddrInfo, requestedFile string) {
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
	defer stream.Close()

	log.Println("[CRDT][runSourceNode] 📤 Sending local metadata for all files...")
	if err := SendMetadataMap(stream, fileMetadataMap); err != nil {
		log.Println("[CRDT][runSourceNode] ❌ Send error:", err)
		return
	}

	remoteMetaMap, err := ReceiveMetadataMap(stream)
	if err != nil {
		log.Println("[CRDT][runSourceNode] ❌ Receive error:", err)
		return
	}

	for name, remoteMeta := range remoteMetaMap {
		localMeta := fileMetadataMap[name]
		fileMetadataMap[name] = MergeFileMetadata(localMeta, remoteMeta)
		log.Printf("[CRDT][runSourceNode] ✅ Merged metadata for file: %s", name)

		if name == requestedFile {
			log.Printf("[CRDT][runSourceNode] 📄 Printing metadata for transferred file: %s", name)
			PrintMetadata(fileMetadataMap[name])
		}
	}
	// Save merged metadata
	if err := saveMetadataToFile("sync-metadata.json"); err != nil {
		log.Printf("[CRDT][runSourceNode] ⚠️ Failed to save metadata to file: %v", err)
	} else {
		log.Println("[CRDT][runSourceNode] 💾 Metadata saved to sync-metadata.json")
	}

}

func readHelloProtocol(s network.Stream) error {
	remoteMetaMap, err := ReceiveMetadataMap(s)
	if err != nil {
		return err
	}

	peerID := s.Conn().RemotePeer()
	log.Printf("[CRDT][readHelloProtocol] 📥 Received metadata map from %s", peerID)

	for name, remoteMeta := range remoteMetaMap {
		localMeta := fileMetadataMap[name]
		fileMetadataMap[name] = MergeFileMetadata(localMeta, remoteMeta)
	}

	if err := SendMetadataMap(s, fileMetadataMap); err != nil {
		return err
	}

	if !syncedPeers[peerID.String()] {
		syncedPeers[peerID.String()] = true
		go func() {
			log.Printf("[CRDT][readHelloProtocol] 🔁 Syncing back to %s", peerID)
			runSourceNode(peer.AddrInfo{ID: peerID}, "") // 👈 no file being requested
		}()
	}

	return nil
}

func main() {
	// Recovery block
	defer func() {
		if r := recover(); r != nil {
			log.Printf("FATAL: Recovered from panic: %v\n", r)
			debug.PrintStack()
			time.Sleep(5 * time.Second)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encryptFlag := flag.Bool("E", false, "Enable AES encryption for file transfer")
	flag.Parse()
	usedEncryption = *encryptFlag

	log.Println("[INIT] 🚀 Starting P2P File Sync Node...")

	// ✅ Create node first
	node = createNode()
	log.Printf("[INIT] 🆔 Peer ID: %s", node.ID().String())

	hostname, _ := os.Hostname()
	log.Printf("[INIT] 🖥️ Hostname: %s", hostname)

	// ✅ Now you can generate versions safely
	files, err := os.ReadDir("shared")
	if err != nil {
		log.Fatalf("[INIT] ❌ Failed to read shared directory: %v", err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fileName := f.Name()
		version := NewFileVersion(node.ID().String(), "initial upload from "+hostname, "CID_"+fileName, nil)

		meta := FileMetadata{
			FileName: fileName,
			Versions: make(map[string]FileVersion),
			Heads:    []string{},
		}
		meta.AddVersion(version)
		fileMetadataMap[fileName] = meta

		log.Printf("[INIT] ✅ File '%s' added to metadata map", fileName)
	}

	// Optional: create a dummy local version for internal syncing
	localFileMetadata = FileMetadata{
		FileName: "sync-metadata.json",
		Versions: make(map[string]FileVersion),
		Heads:    []string{},
	}
	syncVersion := NewFileVersion(node.ID().String(), "initial metadata", "CID123456", nil)
	localFileMetadata.AddVersion(syncVersion)

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
		time.Sleep(15 * time.Second)
		log.Println("[PubSub] 📢 Sending initial file announcement...")
		announceLocalFiles(node.ID().String())
	}()

	startInteractiveCLI(ctx)
	log.Println("[READY] ✅ Node is up and running. Press Ctrl+C to exit.")
	<-ctx.Done()
}
