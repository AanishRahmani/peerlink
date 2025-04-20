# ️ PeerLink P2P File Sharing & Sync System

---

##  Project Overview

This project is a **Peer-to-Peer (P2P)** file synchronization and transfer system built using **Go** and **libp2p**.  
It allows peers to:

- Discover each other automatically on the local network via **mDNS**.
- Share and sync **files and folders**.
- Track **file versions** in a **Git-like CRDT model** for conflict-free updates.
- Transfer **entire directories** while maintaining structure.
- Ensure **integrity** through **SHA-256** hash verification.
- Visualize transfer progress via a terminal **progress bar**.

---

## ️ Key Features

| Feature                                 | Status   |
|:----------------------------------------|:---------|
| Local peer discovery (mDNS)             | ✅       |
| File + Folder sharing (recursive)       | ✅       |
| Progress bars during transfers          | ✅       |
| SHA-256 hash verification               | ✅       |
| Git-like CRDT file versioning (ongoing) | ✅       |
| Interactive CLI for requesting files    | ✅       |

---

## Current Architecture

```
+----------------------------------+
|           Peer Node              |
|----------------------------------|
| 1. Create libp2p host             |
| 2. Start mDNS discovery           |
| 3. Join PubSub topic              |
| 4. Announce available files       |
| 5. Listen for file announcements  |
| 6. Start CLI input loop           |
| 7. Handle incoming file requests  |
| 8. Request files from peers       |
| 9. Perform CRDT version merging   |
+----------------------------------+
```

---

##  Internal Mechanisms

###  File Sharing (Architecture)

```
[SENDER SIDE]                      [RECEIVER SIDE]
shared/ folder                     TransferredFiles/ folder

- docs/                            -> docs/
    - file1.txt                    -> file1.txt
    - file2.txt                    -> file2.txt
- images/                          -> images/
    - photo1.png                   -> photo1.png
- notes.txt                        -> notes.txt
```

1. Sender walks the shared folder recursively.
2. For each file:
   - Send relative path first
   - Stream file data in 4KB chunks
   - Send EOF marker
   - Send SHA-256 file hash
3. Receiver reconstructs directories and files.
4. Receiver verifies the final SHA-256 hash.

---
## Quick Start

1. Install Go 1.20+ prefferably `go1.24.2`
2. Clone the repository.
4. Run `go mod download`.
5. make sure you have a `/shared/` and a `/TransferredFolder/` folder.
6. Put files to be shared inside the `/shared/` folder.
7. Build and run:

```bash

git clone https://github.com/AanishRahmani/peerlink.git
cd peerlink
mkdir shared                  # Files you want to share with the network
mkdir TransferredFiles        # Files you download from other peers
touch sync-metadata.json       # File storing CRDT versioning information (Needs Improvements to be able to handle files on an idividual level )
go mod download
```
5. make sure you have files in the shared folder.

```bash
go build -o peerlink
./peerlink 
```


6. Use the interactive CLI to request files!

---

##  Future Enhancements

- [ ] **Encryption** — Encrypting the data stream using AES-256 along with encrypting the metadata file, the network over which the data is transferred, and the peer-to-peer protocol.
- [ ] **Public security key** — Using a public key for authenticating that the right person is accessing the right network.
- [ ] **RBAC (Role-Based Access Control)** — Using a **Public Access Key** in the mDNS discovery so only authorized peers can join.
- [ ] **Request Queue** (in progress) — Handling multiple simultaneous incoming file requests via a channel.
- [ ] **Queue Management Enhancements** — Smarter backpressure control and retries.
- [ ] **Metadata Sync for every file imdividually** — Improved CRDT for folder structures, not just files kind of like git.
- [ ] **Web UI for File Management** — Beautiful browser-based dashboard.
---

## License

This project includes original work from Protocol Labs.

Additional contributions and a lot of modifications by **Aanish Rahmani** are licensed under the [MIT License](./LICENSES/LICENSE.AANISH).
