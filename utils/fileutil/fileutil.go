package fileutil

import (
	"fmt"
	"os"
)

const (
	// MaxFileSize is the default maximum allowed file size (100MB)
	MaxFileSize = 100 * 1024 * 1024
)

// CheckFileSize verifies if a file is within acceptable size limits
func CheckFileSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error checking file size: %w", err)
	}

	if info.Size() > MaxFileSize {
		return fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes", info.Size(), MaxFileSize)
	}

	return nil
}

// SafeReadFile reads a file after checking its size
func SafeReadFile(path string) ([]byte, error) {
	if err := CheckFileSize(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}
	return data, nil
}

// SafeOpenFile opens a file after checking its size
func SafeOpenFile(path string) (*os.File, error) {
	if err := CheckFileSize(path); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	return file, nil
}
