package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/schollz/progressbar/v3"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const chunkSize = 4096 // 4KB

var useEncryption = false

func handleFileRequest(s network.Stream) {
	defer func(s network.Stream) {
		if err := s.Close(); err != nil {
			log.Printf("[FileTransfer][handleFileRequest] ❌ Error closing stream: %v", err)
		}
	}(s)

	reader := bufio.NewReader(s)

	fileNameRaw, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] ❌ Failed to read file name: %v", err)
		return
	}
	fileName := filepath.Clean(fileNameRaw[:len(fileNameRaw)-1])
	log.Printf("[FileTransfer][handleFileRequest] 📩 File requested: %s", fileName)

	encFlag, err := reader.ReadByte()
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] ❌ Failed to read encryption flag: %v", err)
		return
	}
	peerWantsEncryption := encFlag == 1
	log.Printf("[FileTransfer][handleFileRequest] 🔐 Peer requested %s transfer", encryptionStatus(peerWantsEncryption))

	file, err := os.Open("shared/" + fileName)
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] ❌ File not found: %v", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("[FileTransfer][handleFileRequest] ❌ Error closing file: %v", err)
		}
	}(file)

	hash := sha256.New()
	buf := make([]byte, chunkSize)
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("[FileTransfer][handleFileRequest] ❌ Read error: %v", err)
			return
		}
		if n == 0 {
			break
		}

		data := buf[:n]
		hash.Write(data)

		if peerWantsEncryption {
			data, err = encryptAndCompress(data)
			if err != nil {
				log.Printf("[FileTransfer][handleFileRequest] ❌ Encryption failed: %v", err)
				return
			}
		}

		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
		_, errs := s.Write(lenBuf)
		if errs != nil {
			return
		}
		_, er := s.Write(data)
		if er != nil {
			return
		}
	}

	r := binary.Write(s, binary.BigEndian, uint32(0))
	if r != nil {
		return
	} // EOF
	finalHash := hash.Sum(nil)
	_, errr := s.Write(finalHash)
	if errr != nil {
		return
	}

	log.Printf("[FileTransfer][handleFileRequest] ✅ Sent file '%s' (SHA256: %x)", fileName, finalHash)
	// After file is successfully sent:
	printLock.Lock()
	log.Printf("[Stream][/file-transfer] ✅ File sent to %s", s.Conn().RemotePeer().String())
	showAvailableFiles() // Update display after sending
	printLock.Unlock()
}

func requestFileFromPeer(peerInfo peer.AddrInfo, fileName string) error {
	log.Printf("[FileTransfer][requestFileFromPeer] 📡 Requesting '%s' from peer %s", fileName, peerInfo.ID)

	log.Println("[FileTransfer][requestFileFromPeer] 🔌 Connecting to peer...")
	if err := node.Connect(context.Background(), peerInfo); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Connect failed: %w", err)
	}
	log.Println("[FileTransfer][requestFileFromPeer] ✅ Connected.")

	stream, err := node.NewStream(context.Background(), peerInfo.ID, "/file-transfer/1.0.0")
	if err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Stream failed: %w", err)
	}
	defer func() {
		if err := stream.Close(); err != nil {
			log.Printf("[FileTransfer][requestFileFromPeer] ❌ Error closing stream: %v", err)
		}
	}()

	_, _ = stream.Write([]byte(fileName + "\n"))
	_, _ = stream.Write([]byte{boolToByte(useEncryption)})

	saveDir := filepath.Join(".", "TransferredFiles")
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Failed to create directory: %w", err)
	}

	outputPath := filepath.Join(saveDir, fileName)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Create file failed: %w", err)
	}
	defer func() {
		if err := outputFile.Close(); err != nil {
			log.Printf("[FileTransfer][requestFileFromPeer] ❌ Error closing output file: %v", err)
		}
	}()

	hash := sha256.New()
	reader := bufio.NewReader(stream)

	log.Println("[FileTransfer][requestFileFromPeer] ⏳ Receiving file chunks...")

	startTime := time.Now()
	var chunkCount int64
	var totalBytes int64

	bar := progressbar.NewOptions64(-1,
		progressbar.OptionSetDescription("📦 Receiving"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetPredictTime(true),
		//progressbar.OptionClearOnFinish(),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	barFinished := false
	defer func() {
		if !barFinished {
			_ = bar.Finish()
		}
	}()

	for {
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Length read error: %w", err)
		}
		length := binary.BigEndian.Uint32(lenBuf)

		if length == 0 {
			break // end of transmission
		}

		chunk := make([]byte, length)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Chunk read error: %w", err)
		}

		if useEncryption {
			chunk, err = decryptAndDecompress(chunk)
			if err != nil {
				return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Decrypt error: %w", err)
			}
		}

		hash.Write(chunk)
		_, err := outputFile.Write(chunk)
		if err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Write error: %w", err)
		}

		chunkCount++
		totalBytes += int64(len(chunk))
		_ = bar.Add64(int64(len(chunk)))
	}

	// stop the bar before printing final logs
	_ = bar.Finish()
	barFinished = true
	fmt.Println()

	expectedHash := make([]byte, 32)
	if _, err := io.ReadFull(reader, expectedHash); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Final hash read error: %w", err)
	}
	actualHash := hash.Sum(nil)

	log.Println("[FileTransfer][requestFileFromPeer] 🧮 Verifying SHA-256 hash...")
	if !bytes.Equal(expectedHash, actualHash) {
		log.Printf("[FileTransfer][requestFileFromPeer] ❌ Hash mismatch! Expected: %x, Got: %x", expectedHash, actualHash)
		return fmt.Errorf("hash mismatch")
	}

	elapsed := time.Since(startTime)
	speed := float64(totalBytes) / elapsed.Seconds() / 1024.0

	log.Printf("[FileTransfer][requestFileFromPeer] ✅ File '%s' received and verified", fileName)
	log.Printf("[FileTransfer][requestFileFromPeer] 📁 Saved to: %s", outputPath)
	log.Printf("[FileTransfer][requestFileFromPeer] ⏱️ Duration: %.2fs | Size: %.2f KB | Avg Speed: %.2f KB/s",
		elapsed.Seconds(), float64(totalBytes)/1024.0, speed)

	// Refresh menu after successful transfer

	printLock.Lock()
	showAvailableFiles()
	printLock.Unlock()
	log.Printf("[FileTransfer][requestFileFromPeer] ⏱️ Duration: ...")

	return nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func encryptionStatus(enabled bool) string {
	if enabled {
		return "encrypted"
	}
	return "plaintext"
}
