package main

import (
	"bufio"
	"encoding/json"
	"io"
)

/*

						# OBJECTIVES
1 sending file metadata over stream [DONE]
2 receiving file metadata over stream [DONE]
3 better handling for edge cases --
	- suppose an entire folder is inside the root so handling that

*/

// Send map of FileMetadata over a stream
func SendMetadataMap(w io.Writer, metaMap map[string]FileMetadata) error {
	data, err := json.Marshal(metaMap)
	if err != nil {
		return err
	}
	data = append(data, '\n') // newline to support buffered reader
	_, err = w.Write(data)
	return err
}

// Receive map of FileMetadata from a stream
func ReceiveMetadataMap(r io.Reader) (map[string]FileMetadata, error) {
	reader := bufio.NewReader(r)
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var metaMap map[string]FileMetadata
	err = json.Unmarshal(raw, &metaMap)
	return metaMap, err
}
