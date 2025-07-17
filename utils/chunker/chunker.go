package chunker

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ChunkConfig represents the configuration for chunking a large file
type ChunkConfig struct {
	By        string // How to split the file: "lines", "bytes", or "tokens"
	Size      int    // Chunk size (e.g., 10000 lines)
	Overlap   int    // Lines/bytes to overlap between chunks for context
	MaxChunks int    // Limit total chunks to prevent overload
}

// ChunkResult contains information about the chunking operation
type ChunkResult struct {
	ChunkPaths  []string // Paths to the temporary chunk files
	TempDir     string   // Path to the temporary directory containing the chunks
	TotalChunks int      // Total number of chunks created
}

// SplitFile splits a file into chunks based on the provided configuration
// It returns the paths to the temporary chunk files and a cleanup function
func SplitFile(filePath string, config ChunkConfig) (*ChunkResult, error) {
	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	// Create a temporary directory for the chunks
	tempDir, err := os.MkdirTemp("", "comanda-chunks-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Split the file based on the specified method
	var chunkPaths []string
	var totalChunks int

	switch strings.ToLower(config.By) {
	case "lines":
		chunkPaths, totalChunks, err = splitByLines(filePath, tempDir, config)
	case "bytes":
		chunkPaths, totalChunks, err = splitByBytes(filePath, tempDir, config)
	case "tokens":
		chunkPaths, totalChunks, err = splitByTokens(filePath, tempDir, config)
	default:
		// This should never happen due to validation, but just in case
		return nil, fmt.Errorf("unsupported split method: %s", config.By)
	}

	if err != nil {
		// Clean up the temporary directory if there's an error
		os.RemoveAll(tempDir)
		return nil, err
	}

	return &ChunkResult{
		ChunkPaths:  chunkPaths,
		TempDir:     tempDir,
		TotalChunks: totalChunks,
	}, nil
}

// CleanupChunks removes the temporary directory and all chunk files
func CleanupChunks(result *ChunkResult) error {
	if result == nil || result.TempDir == "" {
		return nil
	}
	return os.RemoveAll(result.TempDir)
}

// validateConfig validates the chunking configuration and sets default values
func validateConfig(config *ChunkConfig) error {
	// Validate the split method
	validMethods := map[string]bool{
		"lines":  true,
		"bytes":  true,
		"tokens": true,
	}

	if !validMethods[strings.ToLower(config.By)] {
		return fmt.Errorf("invalid split method: %s (must be 'lines', 'bytes', or 'tokens')", config.By)
	}

	// Validate the chunk size
	if config.Size <= 0 {
		return fmt.Errorf("chunk size must be greater than 0, got %d", config.Size)
	}

	// Set default values for optional fields
	if config.Overlap < 0 {
		config.Overlap = 0
	}

	if config.MaxChunks <= 0 {
		config.MaxChunks = 100 // Default to 100 chunks maximum
	}

	return nil
}

// splitByLines splits a file into chunks based on line count
func splitByLines(filePath, tempDir string, config ChunkConfig) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var chunkPaths []string
	var lineBuffer []string
	lineCount := 0
	chunkIndex := 0

	// Read all lines into memory
	for scanner.Scan() {
		lineBuffer = append(lineBuffer, scanner.Text())
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading file: %w", err)
	}

	// If the file is empty, return an empty chunk
	if lineCount == 0 {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_0.txt"))
		if err := os.WriteFile(chunkPath, []byte(""), 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write empty chunk: %w", err)
		}
		return []string{chunkPath}, 1, nil
	}

	// Calculate how many chunks we'll create
	totalChunks := (lineCount + config.Size - 1) / config.Size
	if totalChunks > config.MaxChunks {
		return nil, 0, fmt.Errorf("file would generate %d chunks, exceeding the maximum of %d", totalChunks, config.MaxChunks)
	}

	// Create chunks
	for start := 0; start < lineCount; start += config.Size - config.Overlap {
		// Ensure we don't go beyond the maximum number of chunks
		if chunkIndex >= config.MaxChunks {
			break
		}

		end := start + config.Size
		if end > lineCount {
			end = lineCount
		}

		// Create a chunk file
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.txt", chunkIndex))
		chunkContent := strings.Join(lineBuffer[start:end], "\n")

		if err := os.WriteFile(chunkPath, []byte(chunkContent), 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write chunk %d: %w", chunkIndex, err)
		}

		chunkPaths = append(chunkPaths, chunkPath)
		chunkIndex++

		// If we've reached the end of the file, break
		if end >= lineCount {
			break
		}
	}

	return chunkPaths, chunkIndex, nil
}

// splitByBytes splits a file into chunks based on byte size
func splitByBytes(filePath, tempDir string, config ChunkConfig) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get file info: %w", err)
	}
	fileSize := fileInfo.Size()

	// If the file is empty, return an empty chunk
	if fileSize == 0 {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_0.txt"))
		if err := os.WriteFile(chunkPath, []byte(""), 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write empty chunk: %w", err)
		}
		return []string{chunkPath}, 1, nil
	}

	// Calculate how many chunks we'll create
	totalChunks := (int(fileSize) + config.Size - 1) / config.Size
	if totalChunks > config.MaxChunks {
		return nil, 0, fmt.Errorf("file would generate %d chunks, exceeding the maximum of %d", totalChunks, config.MaxChunks)
	}

	var chunkPaths []string
	chunkIndex := 0

	// Create chunks
	for offset := int64(0); offset < fileSize; offset += int64(config.Size - config.Overlap) {
		// Ensure we don't go beyond the maximum number of chunks
		if chunkIndex >= config.MaxChunks {
			break
		}

		// Calculate the end position for this chunk
		end := offset + int64(config.Size)
		if end > fileSize {
			end = fileSize
		}

		// Create a buffer for this chunk
		chunkSize := end - offset
		buffer := make([]byte, chunkSize)

		// Seek to the offset and read the chunk
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to seek to position %d: %w", offset, err)
		}

		_, err = io.ReadFull(file, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, 0, fmt.Errorf("failed to read chunk at position %d: %w", offset, err)
		}

		// Create a chunk file
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.txt", chunkIndex))
		if err := os.WriteFile(chunkPath, buffer, 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write chunk %d: %w", chunkIndex, err)
		}

		chunkPaths = append(chunkPaths, chunkPath)
		chunkIndex++

		// If we've reached the end of the file, break
		if end >= fileSize {
			break
		}
	}

	return chunkPaths, chunkIndex, nil
}

// splitByTokens splits a file into chunks based on approximate token count
// This is a simple implementation that uses whitespace as a token delimiter
func splitByTokens(filePath, tempDir string, config ChunkConfig) ([]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var tokens []string
	var tokenCount int

	// Read the file and split into tokens
	for scanner.Scan() {
		line := scanner.Text()
		lineTokens := strings.Fields(line)
		tokens = append(tokens, lineTokens...)
		tokenCount += len(lineTokens)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading file: %w", err)
	}

	// If the file is empty, return an empty chunk
	if tokenCount == 0 {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_0.txt"))
		if err := os.WriteFile(chunkPath, []byte(""), 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write empty chunk: %w", err)
		}
		return []string{chunkPath}, 1, nil
	}

	// Calculate how many chunks we'll create
	totalChunks := (tokenCount + config.Size - 1) / config.Size
	if totalChunks > config.MaxChunks {
		return nil, 0, fmt.Errorf("file would generate %d chunks, exceeding the maximum of %d", totalChunks, config.MaxChunks)
	}

	var chunkPaths []string
	chunkIndex := 0

	// Create chunks
	for start := 0; start < tokenCount; start += config.Size - config.Overlap {
		// Ensure we don't go beyond the maximum number of chunks
		if chunkIndex >= config.MaxChunks {
			break
		}

		end := start + config.Size
		if end > tokenCount {
			end = tokenCount
		}

		// Create a chunk file
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.txt", chunkIndex))
		chunkContent := strings.Join(tokens[start:end], " ")

		if err := os.WriteFile(chunkPath, []byte(chunkContent), 0644); err != nil {
			return nil, 0, fmt.Errorf("failed to write chunk %d: %w", chunkIndex, err)
		}

		chunkPaths = append(chunkPaths, chunkPath)
		chunkIndex++

		// If we've reached the end of the file, break
		if end >= tokenCount {
			break
		}
	}

	return chunkPaths, chunkIndex, nil
}
