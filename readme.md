# 🔁 Decentralized File Sync with libp2p

A simple, hackable, peer-to-peer file synchronization tool built using [libp2p](https://libp2p.io). Designed for local and lightweight global use without blockchain, tokens, or heavy protocols.

This is a **developer-first**, **CLI-native**, **LAN-aware** tool that syncs files and metadata across nodes using CRDT-style version merging and libp2p streams.

> ⚠️ This project is a heavily modified and extended version of Protocol Labs' [`libp2p-go-handlers`](https://github.com/protocol/launchpad-tutorials/tree/main/libp2p-go-handlers).  
> See [`NOTICE.md`](./NOTICE.md) for terms of use.

---

## ✨ Features

- ✅ File sharing via encrypted libp2p streams
- ✅ CRDT-style metadata merging (automatic)
- ✅ LAN peer discovery using mDNS
- ✅ PubSub-based file announcements
- ✅ Chunked file transfers with SHA256 verification
- ✅ Stream-based architecture (no global FS)
- ✅ CLI-based interface with interactive sync requests
- ✅ Optional AES encryption for transfers
- ✅ Lightweight and beginner-hackable codebase

---



## 🚀 Getting Started

1. Clone the repo:

   ```bash
   git clone https://github.com/yourname/libp2p-decentralized-sync
   cd libp2p-decentralized-sync
