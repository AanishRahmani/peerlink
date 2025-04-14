package main

import (
	"encoding/json"
	"fmt"
	"os"
)

/*

					# OBJECTIVES
1 every file in root directory should have metadata related to them
2 every file in shared and transferred folder should have metadata related to them too
3 a git like implementation should be their for back tracking
4 a .metadata file should be generated too in the begging for storage purpose


					# HOPES
#	Making something like a DLT for files inside [shared folder (or maybe making the root folder as shared that is any file inside the root folder can be shared)]
	except for .shareignore file kind of like git ignore


*/

// Save metadata to sync-metadata.json
func saveMetadataToFile(path string) error {
	data, err := json.MarshalIndent(fileMetadataMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write metadata to file: %w", err)
	}
	return nil
}

// better architecture required
func loadMetadataFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return json.Unmarshal(data, &fileMetadataMap)
}
