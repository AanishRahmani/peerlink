package main

import (
	"bufio"
	"encoding/json"
	"io"
)

// Send FileMetadata over a libp2p stream
func SendMetadata(w io.Writer, meta FileMetadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	data = append(data, '\n') // newline for reader compatibility
	_, err = w.Write(data)
	return err
}

// Receive FileMetadata from a libp2p stream
func ReceiveMetadata(r io.Reader) (FileMetadata, error) {
	var meta FileMetadata
	reader := bufio.NewReader(r)
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(raw, &meta)
	return meta, err
}
