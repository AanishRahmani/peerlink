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
	"strings"
	"time"
)

/*

							# OBJECTIVES
1 handling file request (sending a file)
	- sends 4kb chunks of data (same as page default) to network stream [DONE]
	- Sends a 0-length chunk (EOF marker) and the final SHA-256 hash for integrity.[DONE]
2 request file from peer (receiving file)
	- establish a stream with target peer [DONE]
	- send the requested file [DONE]
	- receive and reconstruct inside transferredfile folder
	- uses a progressbar to visually indicate transfer.
	- verifies the SHA-256 hash to detect corruption.
	- prints download stats and refreshes the file listing.


-------------------------------------------------------------------------

				# architecture for sharing files and folders

[SENDER SIDE]                                  [RECEIVER SIDE]
(shared/ folder)                               (TransferredFiles/ folder)
  |
  |-- docs/                                   ->   docs/
  |     |-- file1.txt                         ->     file1.txt
  |     |-- file2.txt                         ->     file2.txt
  |
  |-- images/                                 ->   images/
  |     |-- photo1.png                        ->     photo1.png
  |
  |-- notes.txt                               ->   notes.txt


						# internal flow of data

 SENDER                                RECEIVER
---------                             ------------
1. send path length (4 bytes)    ->   read 4 bytes (uint32)
2. send relative path            ->   read path

LOOP:
3. send chunk length (4 bytes)   ->   read 4 bytes (chunk size)
4. send chunk data               ->   read chunk data
(repeat)

5. send 0 length (EOF)            ->   read 0 (know file ended)
6. send SHA256 hash              ->   read 32 bytes hash
                                    ->   compare local vs remote hash


				# architecture for checking file integrity
Sender                         Receiver
-----------                    --------
read chunk ---> send chunk ------------> read chunk
           |-> hash.Write()         |---> hash.Write()

   [EOF] -> send 0 uint32            <--- read 0 uint32 (EOF)
          send SHA256 hash          <--- read SHA256 hash
                                     |---- compare hashes


Process:
 For each file:
   1. Send relative path (e.g., "docs/file1.txt")
   2. Send file content in chunks
   3. After file is complete, send EOF marker
   4. Send SHA256 hash for integrity verification

Receiver:
 - Reads relative path
 - Creates folders if needed
 - Writes chunks into the corresponding file
 - Verifies file integrity using SHA256


--------------------------------------------------------------------------

*/

const chunkSize = 4096 // 4KB

var useEncryption = false

func sendSingleFile(s network.Stream, filePath string, peerWantsEncryption bool) error {
	relPath, err := filepath.Rel("shared", filePath)
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile] Failed to calculate relative path: %v", err)
	}

	// ✨ First send the relative path
	pathBytes := []byte(relPath)
	pathLenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBuf, uint32(len(pathBytes)))

	_, err = s.Write(pathLenBuf)
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile] Failed writing path length: %v", err)
	}
	_, err = s.Write(pathBytes)
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile]Failed writing path: %v", err)
	}

	// ✨ Now normal file content sending
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile]  Cannot open file: %v", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	hash := sha256.New()
	buf := make([]byte, chunkSize)

	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("[FileTransfer][sendSingleFile] Read error: %v", err)
		}
		if n == 0 {
			break
		}

		data := buf[:n]
		hash.Write(data)

		if peerWantsEncryption {
			data, err = encryptAndCompress(data)
			if err != nil {
				return fmt.Errorf("[FileTransfer][sendSingleFile] Encryption failed: %v", err)
			}
		}

		lenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
		_, err = s.Write(lenBuf)
		if err != nil {
			return fmt.Errorf("[FileTransfer][sendSingleFile] Stream write error: %v", err)
		}
		_, err = s.Write(data)
		if err != nil {
			return fmt.Errorf("[FileTransfer][sendSingleFile] tream write error: %v", err)
		}
	}

	// EOF marker
	err = binary.Write(s, binary.BigEndian, uint32(0))
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile] Stream EOF write error: %v", err)
	}

	finalHash := hash.Sum(nil)
	_, err = s.Write(finalHash)
	if err != nil {
		return fmt.Errorf("[FileTransfer][sendSingleFile] Final hash send error: %v", err)
	}

	return nil
}

func sendFolderContents(s network.Stream, folderPath string, peerWantsEncryption bool) error {
	return filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[FileTransfer][sendFolderContents] Walk error: %v", err)
			return nil
		}

		if info.IsDir() {
			return nil // Skip folders themselves
		}

		log.Printf("[FileTransfer][sendFolderContents] Sending file inside folder: %s", path)
		return sendSingleFile(s, path, peerWantsEncryption)
	})
}

func handleFileRequest(s network.Stream) {
	defer func(s network.Stream) {
		if err := s.Close(); err != nil {
			log.Printf("[FileTransfer][handleFileRequest]❌ Error closing stream: %v", err)
		}
	}(s)

	reader := bufio.NewReader(s)

	fileNameRaw, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] Failed to read file name: %v", err)
		return
	}
	requestedPath := filepath.Clean(fileNameRaw[:len(fileNameRaw)-1])
	log.Printf("[FileTransfer][handleFileRequest] File/Folder requested: %s", requestedPath)

	encFlag, err := reader.ReadByte()
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] Failed to read encryption flag: %v", err)
		return
	}
	peerWantsEncryption := encFlag == 1
	log.Printf("[FileTransfer][handleFileRequest] Peer requested %s transfer", encryptionStatus(peerWantsEncryption))

	// Find and handle file or folder
	rootPath := filepath.Join("shared", requestedPath)
	info, err := os.Stat(rootPath)
	if err != nil {
		log.Printf("[FileTransfer][handleFileRequest] Requested item not found: %v", err)
		return
	}

	if info.IsDir() {
		log.Printf("[FileTransfer][handleFileRequest] Folder requested, sending contents recursively...")
		err = sendFolderContents(s, rootPath, peerWantsEncryption)
		if err != nil {
			log.Printf("[FileTransfer][handleFileRequest] ❌ Failed to send folder: %v", err)
		}
	} else {
		log.Printf("[FileTransfer][handleFileRequest] Single file requested, sending...")
		err = sendSingleFile(s, rootPath, peerWantsEncryption)
		if err != nil {
			log.Printf("[FileTransfer][handleFileRequest] Failed to send file: %v", err)
		}
	}

	log.Printf("[FileTransfer][handleFileRequest] Completed transfer for %s", requestedPath)
	printLock.Lock()
	showAvailableFiles()
	printLock.Unlock()
}

func isStreamCancelError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "canceled stream")
}

func requestFileFromPeer(peerInfo peer.AddrInfo, fileName string) error {
	log.Printf("[FileTransfer][requestFileFromPeer] Requesting '%s' from peer %s", fileName, peerInfo.ID)

	log.Println("[FileTransfer][requestFileFromPeer] Connecting to peer...")
	if err := node.Connect(context.Background(), peerInfo); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] Connect failed: %w", err)
	}
	log.Println("[FileTransfer][requestFileFromPeer] Connected.")

	stream, err := node.NewStream(context.Background(), peerInfo.ID, "/file-transfer/1.0.0")
	if err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] Stream creation failed: %w", err)
	}
	defer func() {
		if cerr := stream.Close(); cerr != nil && !isStreamCancelError(cerr) {
			log.Printf("[FileTransfer][requestFileFromPeer] Error closing stream: %v", cerr)
		}
	}()

	// Send the requested file/folder name
	if _, err := stream.Write([]byte(fileName + "\n")); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Failed to send filename: %w", err)
	}
	if _, err := stream.Write([]byte{boolToByte(useEncryption)}); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] Failed to send encryption flag: %w", err)
	}

	reader := bufio.NewReader(stream)

	saveDir := filepath.Join(".", "TransferredFiles")
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		return fmt.Errorf("[FileTransfer][requestFileFromPeer] Could not create TransferredFiles directory: %w", err)
	}

	log.Println("[FileTransfer][requestFileFromPeer] Receiving file(s)...")

	startTime := time.Now()
	var totalBytes int64

	bar := progressbar.NewOptions64(-1,
		progressbar.OptionSetDescription("📦 Receiving"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
	defer func() {
		_ = bar.Finish()
		fmt.Println()
	}()

	for {
		// 1️⃣ Read path length
		pathLenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, pathLenBuf); err != nil {
			break // likely EOF: no more files
		}
		pathLen := binary.BigEndian.Uint32(pathLenBuf)

		if pathLen == 0 {
			break // clean termination
		}

		// 2️⃣ Read path bytes
		pathBytes := make([]byte, pathLen)
		if _, err := io.ReadFull(reader, pathBytes); err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] ❌ Path read error: %w", err)
		}
		relativePath := string(pathBytes)
		log.Printf("[FileTransfer][requestFileFromPeer] Receiving: %s", relativePath)

		outputPath := filepath.Join(saveDir, relativePath)

		// Create parent folders if needed
		if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] Creating directories failed: %w", err)
		}

		outputFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] File create failed: %w", err)
		}

		hash := sha256.New()

		// 3️⃣ Read file chunks
		for {
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(reader, lenBuf); err != nil {
				err := outputFile.Close()
				if err != nil {
					return err
				}
				return fmt.Errorf("[FileTransfer][requestFileFromPeer] Chunk length read error: %w", err)
			}
			chunkLen := binary.BigEndian.Uint32(lenBuf)

			if chunkLen == 0 {
				break // end of the current file
			}

			chunk := make([]byte, chunkLen)
			if _, err := io.ReadFull(reader, chunk); err != nil {
				err := outputFile.Close()
				if err != nil {
					return err
				}
				return fmt.Errorf("[FileTransfer][requestFileFromPeer] Chunk read error: %w", err)
			}

			if useEncryption {
				chunk, err = decryptAndDecompress(chunk)
				if err != nil {
					err := outputFile.Close()
					if err != nil {
						return err
					}
					return fmt.Errorf("[FileTransfer][requestFileFromPeer] Decryption failed: %w", err)
				}
			}

			hash.Write(chunk)
			if _, err := outputFile.Write(chunk); err != nil {
				err := outputFile.Close()
				if err != nil {
					return err
				}
				return fmt.Errorf("[FileTransfer][requestFileFromPeer] Write to file failed: %w", err)
			}

			totalBytes += int64(len(chunk))
			_ = bar.Add64(int64(len(chunk)))
		}

		errs := outputFile.Close()
		if errs != nil {
			return errs
		}

		// 4️⃣ After EOF marker, verify SHA256
		expectedHash := make([]byte, 32)
		if _, err := io.ReadFull(reader, expectedHash); err != nil {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] Final hash read error: %w", err)
		}

		actualHash := hash.Sum(nil)
		if !bytes.Equal(expectedHash, actualHash) {
			return fmt.Errorf("[FileTransfer][requestFileFromPeer] Hash mismatch on file %s", relativePath)
		}

		log.Printf("[FileTransfer][requestFileFromPeer] File '%s' verified", relativePath)
	}

	elapsed := time.Since(startTime)
	speed := float64(totalBytes) / elapsed.Seconds() / 1024.0

	log.Printf("[FileTransfer][requestFileFromPeer] Saved all files under: %s", saveDir)
	log.Printf("[FileTransfer][requestFileFromPeer] Duration: %.2fs | Size: %.2f KB | Avg Speed: %.2f KB/s",
		elapsed.Seconds(), float64(totalBytes)/1024.0, speed)

	//printLock.Lock()
	//showAvailableFiles()
	//printLock.Unlock()

	return nil
}

func encryptionStatus(enabled bool) string {
	if enabled {
		return "encrypted"
	}
	return "plaintext"
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
