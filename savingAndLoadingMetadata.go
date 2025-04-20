package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

/*
──────────────────────────────────────────────────────────────────────────────
                              # OBJECTIVES

1. Every file inside shared/transferred folder must have associated metadata
2. Full Git-like tracking support (CRDT + versions)
3. .metadata file will act like a lightweight DLT ledger
4. Ignore files like .shareignore can be added later (TODO)

──────────────────────────────────────────────────────────────────────────────
                              # NOTES

- fileMetadataMap = map[string]FileMetadata (global)
- All operations (save/load) are based on this in-memory map
- Future extensibility: Multiple folders, ignore patterns, partial syncs
──────────────────────────────────────────────────────────────────────────────
*/

// Save current file metadata map into a metadata file
func saveMetadataToFile(path string) error {
	// Ensure directory exists
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return fmt.Errorf("[savingAndLoadingMetaData][saveMetaDataToFile]failed to create metadata folder: %w", err)
	}

	// Marshal the map into a nicely formatted JSON
	data, err := json.MarshalIndent(fileMetadataMap, "", "  ")
	if err != nil {
		return fmt.Errorf("[savingAndLoadingMetaData][saveMetaDataToFile] failed to marshal metadata: %w", err)
	}

	// Write the JSON to disk
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("[savingAndLoadingMetaData][saveMetaDataToFile] failed to write metadata to file: %w", err)
	}

	return nil
}

// Load file metadata map from a metadata file (if exists)
func loadMetadataFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		// If file not found, treat as empty metadata (not fatal error)
		if os.IsNotExist(err) {
			fileMetadataMap = make(map[string]FileMetadata)
			return nil
		}
		return fmt.Errorf("[savingAndLoadingMetaData[loadMetaDataFromFile]failed to read metadata file: %w", err)
	}

	// Decode JSON back into the global map
	return json.Unmarshal(data, &fileMetadataMap)
}

// (Optional Helper) Initialize .metadata file if missing
func ensureMetadataFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// No metadata exists, create a blank one
		fileMetadataMap = make(map[string]FileMetadata)
		return saveMetadataToFile(path)
	}
	return nil // Metadata file already exists
}
