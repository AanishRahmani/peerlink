package main

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

// ⚠️ You should replace this with a group-based secret key (32 bytes = 256 bits)
var encryptionKey = []byte("this_is_a_demo_key_32_bytes_long!!") // 32 bytes

// Compress data with gzip and encrypt it using AES-256-GCM
func encryptAndCompress(input []byte) ([]byte, error) {
	// Compress
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(input)
	if err != nil {
		return nil, err
	}
	writer.Close()

	// Encrypt
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	encrypted := gcm.Seal(nonce, nonce, compressed.Bytes(), nil)
	return encrypted, nil
}

// Decrypt AES-256-GCM encrypted data and decompress it using gzip
func decryptAndDecompress(input []byte) ([]byte, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(input) < nonceSize {
		return nil, io.ErrUnexpectedEOF
	}

	nonce, ciphertext := input[:nonceSize], input[nonceSize:]
	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Decompress
	reader, err := gzip.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var decompressed bytes.Buffer
	if _, err := io.Copy(&decompressed, reader); err != nil {
		return nil, err
	}

	return decompressed.Bytes(), nil
}
