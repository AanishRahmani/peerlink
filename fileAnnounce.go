package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

/*

This file manages how peers-
    - join the PubSub topic[DONE]
    - broadcast what files they are offering[DONE]
    - listen for other peers' announcements[DONE]
    - keep an updated list of available files across the network[DONE]
*/

var (
	fileTopic      *pubsub.Topic
	fileSub        *pubsub.Subscription
	knownFiles     = make(map[string][]string)
	knownFilesLock sync.Mutex
)

type FileAnnouncement struct {
	PeerID   string   `json:"peer_id"`
	FileList []string `json:"file_list"`
}

// Join pubsub topic and start listening
func setupFilePubSub(ctx context.Context, ps *pubsub.PubSub, peerID string) error {
	var err error
	fileTopic, err = ps.Join("file-presence")
	if err != nil {
		log.Printf("[PubSub][setupFilePubSub] ❌ Failed to join topic: %v", err)
		return err
	}
	log.Println("[PubSub][setupFilePubSub] ✅ Joined pubsub topic 'file-presence'")

	fileSub, err = fileTopic.Subscribe()
	if err != nil {
		log.Printf("[PubSub][setupFilePubSub] ❌ Failed to subscribe: %v", err)
		return err
	}
	log.Println("[PubSub][setupFilePubSub] ✅ Subscribed to file announcements")

	go listenForAnnouncements(ctx, peerID)
	return nil
}

// Announce local files in ./shared
func announceLocalFiles(peerID string) {
	files, err := os.ReadDir("./shared")
	if err != nil {
		log.Printf("[PubSub][announceLocalFiles] ❌ Could not read shared folder: %v", err)
		return
	}

	var fileNames []string
	for _, f := range files {
		if !f.IsDir() {
			fileNames = append(fileNames, f.Name())
		}
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
		log.Printf("[PubSub][announceLocalFiles] ❌ Failed to publish: %v", err)
		return
	}

	log.Printf("[Announce] 📢 Announced files: %v", fileNames)
}

// Listen for incoming file announcements
func listenForAnnouncements(ctx context.Context, selfID string) {
	log.Println("[PubSub][listenForAnnouncements] 🔄 Listening for file announcements...")

	for {
		msg, err := fileSub.Next(ctx)
		if err != nil {
			log.Printf("[PubSub][listenForAnnouncements] ❌ PubSub error: %v", err)
			return
		}

		if msg.ReceivedFrom.String() == selfID {
			continue
		}

		var ann FileAnnouncement
		if err := json.Unmarshal(msg.Data, &ann); err != nil {
			log.Printf("[PubSub][listenForAnnouncements] ❌ Failed to parse announcement: %v", err)
			continue
		}

		log.Printf("[PubSub][listenForAnnouncements] 📥 Received announcement from %s", ann.PeerID)

		knownFilesLock.Lock()
		knownFiles[ann.PeerID] = ann.FileList
		knownFilesLock.Unlock()

		log.Printf("[Announce] Peer %s offers: %v", ann.PeerID, ann.FileList)
		printLock.Lock()
		showAvailableFiles()
		//fmt.Println("📁 Enter file name to download, '' to re-announce (leave string empty and press enter), or press Ctrl+C to exit:")
		//fmt.Print("> ")
		printLock.Unlock()
	}
}

// Show current known file offerings
func showAvailableFiles() {

	fmt.Println("📂 Available Files:")
	if len(knownFiles) == 0 {
		fmt.Println("   (No files announced yet)")
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
