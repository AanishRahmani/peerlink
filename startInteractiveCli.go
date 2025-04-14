package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

/*

				# OBJECTIVES
1 starts interactive CLI [DONE]
2 View available files[DONE]
3 Request a file from a discovered peer[DONE]
4 Trigger a re-announcement[DONE]
5 Exit cleanly on cancellation[DONE]
*/

func startInteractiveCLI(ctx context.Context) {
	go func() {
		time.Sleep(16 * time.Second)
		reader := bufio.NewReader(os.Stdin)

		for {
			select {
			case <-ctx.Done():
				log.Println("[CLI] 🚪 Exiting interactive CLI due to context cancel.")
				return
			default:
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
		}
	}()
}
