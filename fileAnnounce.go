package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

/*
This file manages how peers:
    - join the PubSub topic [DONE]
    - broadcast what files/folders they are offering [UPDATED ]
    - listen for other peers' announcements [DONE]
    - keep an updated list of available files/folders across the network [UPDATED ]
*/

var (
	fileTopic      *pubsub.Topic
	fileSub        *pubsub.Subscription
	knownFiles     = make(map[string][]string) // peerID → list of offered files/folders
	knownFilesLock sync.Mutex
)

type FileAnnouncement struct {
	PeerID   string   `json:"peer_id"`
	FileList []string `json:"file_list"`
}

// Setup PubSub: Join topic and start listening
func setupFilePubSub(ctx context.Context, ps *pubsub.PubSub, peerID string) error {
	var err error
	fileTopic, err = ps.Join("file-presence")
	if err != nil {
		log.Printf("[PubSub][setupFilePubSub] Failed to join topic: %v", err)
		return err
	}
	log.Println("[PubSub][setupFilePubSub] Joined pubsub topic 'file-presence'")

	fileSub, err = fileTopic.Subscribe()
	if err != nil {
		log.Printf("[PubSub][setupFilePubSub] Failed to subscribe: %v", err)
		return err
	}
	log.Println("[PubSub][setupFilePubSub] Subscribed to file announcements")

	go listenForAnnouncements(ctx, peerID)
	return nil
}

// Announce local files and folders in ./shared
func announceLocalFiles(peerID string) {
	var fileNames []string

	// Recursively walk through 'shared' directory
	err := filepath.Walk("shared", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[PubSub][announceLocalFiles] Walk error: %v", err)
			return nil
		}

		// Skip root 'shared/' itself
		if path == "shared" {
			return nil
		}

		// Always store relative path
		relPath, err := filepath.Rel("shared", path)
		if err != nil {
			log.Printf("[PubSub][announceLocalFiles] RelPath error: %v", err)
			return nil
		}

		if info.IsDir() {
			// Optional: Also announce folders
			fileNames = append(fileNames, relPath+"/") // Folder ends with slash
		} else {
			fileNames = append(fileNames, relPath)
		}
		return nil
	})
	if err != nil {
		log.Printf("[PubSub][announceLocalFiles] Walk error: %v", err)
		return
	}

	msg := FileAnnouncement{
		PeerID:   peerID,
		FileList: fileNames,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[PubSub][announceLocalFiles] ❌ JSON encode failed: %v", err)
		return
	}

	if err := fileTopic.Publish(context.Background(), data); err != nil {
		log.Printf("[PubSub][announceLocalFiles] Failed to publish: %v", err)
		return
	}

	log.Printf("[Announce] Announced files/folders: %v", fileNames)
}

// Listen for incoming file/folder announcements
func listenForAnnouncements(ctx context.Context, selfID string) {
	log.Println("[PubSub][listenForAnnouncements] Listening for file announcements...")

	for {
		msg, err := fileSub.Next(ctx)
		if err != nil {
			log.Printf("[PubSub][listenForAnnouncements] PubSub error: %v", err)
			return
		}

		// Ignore our own announcements
		if msg.ReceivedFrom.String() == selfID {
			continue
		}

		var ann FileAnnouncement
		if err := json.Unmarshal(msg.Data, &ann); err != nil {
			log.Printf("[PubSub][listenForAnnouncements] Failed to parse announcement: %v", err)
			continue
		}

		log.Printf("[PubSub][listenForAnnouncements] Received announcement from %s", ann.PeerID)

		knownFilesLock.Lock()
		knownFiles[ann.PeerID] = ann.FileList
		knownFilesLock.Unlock()

		log.Printf("[Announce] Peer %s offers: %v", ann.PeerID, ann.FileList)

		printLock.Lock()
		showAvailableFiles()
		printLock.Unlock()
	}
}

// Show available files/folders from all known peers
func showAvailableFiles() {
	fmt.Println("📂 Available Files/Folders:")
	if len(knownFiles) == 0 {
		fmt.Println("   (No announcements yet)")
		return
	}

	for peerID, files := range knownFiles {
		fmt.Printf("🧑 %s:\n", peerID)
		if len(files) == 0 {
			fmt.Println("   (No files listed by this peer)")
		}
		for _, f := range files {
			fmt.Printf("   - %s\n", f)
		}
	}
}
