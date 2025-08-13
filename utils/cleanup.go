package utils

import "os"

// CleanupOldFiles removes the entire output directory and all its contents
func CleanupOldFiles(outputFile string) {
	// Remove entire output directory and all its contents
	os.RemoveAll("output")
}

// EnsureDirectories creates necessary output directories
func EnsureDirectories() error {
	if err := os.MkdirAll("output/assets", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("output/assets/images", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("output/assets/fonts", 0755); err != nil {
		return err
	}
	return nil
}