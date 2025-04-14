package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"
)

/*
 # handling CRDT(map type) in a GIT way of doing things conflict free way to handle data consistency

----- 	This file implements a Git-inspired CRDT (Conflict-free Replicated Data Type) system to track and synchronize file versions in a
decentralized peer-to-peer (P2P) network. It ensures consistency of file metadata across peers without requiring a central server
or manual conflict resolution.	-----

								# OBJECTIVES
1 creating file versions [DONE][STRUCT]
2 creating file metadata  [DONE][STRUCT]
3 create a new file version for each file that is in the shared folder (thinking of a better implementation) [DONE for now]
4 generate a sha256 code for the file that is scanned for verification purpose done [DONE]
5 merging file metadata for both local and globally available files [DONE]
6 update the head properly [DONE]
7 sync metadata efficiently during handshake (during /hello protocol)[DONE]
8 add an appropriate file version


*/

type FileVersion struct {
	VersionID string    `json:"version_id"` // SHA256 hash of this version (computed from all fields)
	ParentIDs []string  `json:"parent_ids"` // One or more parent versions (for merge support)
	Author    string    `json:"author"`     // Peer ID of who made this version
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"` // Optional log
	CID       string    `json:"cid"`     // IPFS CID or file hash
}

// FileMetadata represents metadata for a file with multiple versions
type FileMetadata struct {
	FileName string                 `json:"file_name"`
	Versions map[string]FileVersion `json:"versions"` // versionID → version
	Heads    []string               `json:"heads"`    // latest versions (can have multiple for forks)
}

func NewFileVersion(author, message, cid string, parents []string) FileVersion {
	log.Printf("[crdt][NewFileVersion] creating new version for CID %s by %s", cid, author)
	timeStamp := time.Now().UTC()
	version := FileVersion{
		ParentIDs: parents,
		Author:    author,
		Timestamp: timeStamp,
		Message:   message,
		CID:       cid,
	}
	version.VersionID = GenerateHash(version)
	log.Printf("[crdt][NewFileVersion] new version ID: %s", version.VersionID)
	return version
}

func GenerateHash(version FileVersion) string {
	log.Printf("[crdt][GenerateHash] generating hash for version: %s", version.Message)
	data := fmt.Sprintf("%v|%s|%s|%s|%s",
		version.ParentIDs,
		version.Author,
		version.Timestamp.String(),
		version.Message,
		version.CID,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func MergeFileMetadata(local, remote FileMetadata) FileMetadata {
	log.Printf("[crdt][MergeFileMetadata] merging metadata for file: %s", local.FileName)
	merged := FileMetadata{
		FileName: local.FileName,
		Versions: make(map[string]FileVersion),
		Heads:    []string{},
	}

	log.Printf("[crdt][MergeFileMetadata] copying local versions")
	for k, v := range local.Versions {
		merged.Versions[k] = v
	}
	log.Printf("[crdt][MergeFileMetadata] copying remote versions")
	for k, v := range remote.Versions {
		merged.Versions[k] = v
	}

	log.Printf("[crdt][MergeFileMetadata] combining heads from local and remote")
	headSet := make(map[string]struct{})
	for _, h := range append(local.Heads, remote.Heads...) {
		headSet[h] = struct{}{}
	}
	for h := range headSet {
		merged.Heads = append(merged.Heads, h)
	}

	// Optional: sort heads for consistent output
	sort.Strings(merged.Heads)
	log.Printf("[crdt][MergeFileMetadata] merge complete with heads: %v", merged.Heads)

	return merged
}

func (f *FileMetadata) AddVersion(version FileVersion) {
	log.Printf("[crdt][AddVersion] adding version %s to file %s", version.VersionID, f.FileName)
	if f.Versions == nil {
		log.Printf("[crdt][AddVersion] initializing Versions map")
		f.Versions = make(map[string]FileVersion)
	}
	f.Versions[version.VersionID] = version

	log.Printf("[crdt][AddVersion] updating heads after adding version")
	newHeads := make([]string, 0)
	parentSet := make(map[string]struct{})
	for _, pid := range version.ParentIDs {
		parentSet[pid] = struct{}{}
	}
	for _, head := range f.Heads {
		if _, isParent := parentSet[head]; !isParent {
			newHeads = append(newHeads, head)
		}
	}
	newHeads = append(newHeads, version.VersionID)
	f.Heads = newHeads
	log.Printf("[crdt][AddVersion] new heads: %v", f.Heads)
}

// Pretty-print metadata as JSON
func PrintMetadata(meta FileMetadata) {
	// log.Printf("[crdt][PrintMetadata] printing metadata for file %s", meta.FileName)
	b, _ := json.MarshalIndent(meta, "", "  ")
	fmt.Println(string(b))
}
