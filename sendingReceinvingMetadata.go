package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

/*
──────────────────────────────────────────────────────────────────────────────
                               # OBJECTIVES

1. Send file metadata maps over network streams  [DONE]
2. Receive file metadata maps from network streams [DONE]
3. Edge case handling:
  - Large metadata support (buffered reading) [DONE]
  - Handle folders inside root if needed later (already possible) [DONE]

──────────────────────────────────────────────────────────────────────────────
*/

// SendMetadataMap serializes and sends a map of FileMetadata over an io.Writer (stream)
func SendMetadataMap(w io.Writer, metaMap map[string]FileMetadata) error {
	data, err := json.Marshal(metaMap)
	if err != nil {
		return fmt.Errorf("[sendingReceivingMetadata][SendMetadataMap] JSON marshal error: %w", err)
	}

	// Add newline so buffered reader can detect EOF cleanly
	data = append(data, '\n')

	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("[sendingReceivingMetadata][SendMetadataMap] Stream write error after %d bytes: %w", n, err)
	}

	return nil
}

// ReceiveMetadataMap reads and parses a map of FileMetadata from an io.Reader (stream)
func ReceiveMetadataMap(r io.Reader) (map[string]FileMetadata, error) {
	reader := bufio.NewReader(r)

	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("[sendingReceivingMetadata][ReceiveMetadataMap] Stream read error: %w", err)
	}

	var metaMap map[string]FileMetadata
	err = json.Unmarshal(raw, &metaMap)
	if err != nil {
		return nil, fmt.Errorf("[sendingReceivingMetadata][ReceiveMetadataMap] JSON unmarshal error: %w", err)
	}

	return metaMap, nil
}
