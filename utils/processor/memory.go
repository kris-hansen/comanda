package processor

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// MemoryManager handles reading from and writing to the COMANDA.md memory file
type MemoryManager struct {
	filePath string
	content  string
	mu       sync.RWMutex // Thread-safe for parallel step execution
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(filePath string) (*MemoryManager, error) {
	m := &MemoryManager{
		filePath: filePath,
	}

	// Load initial content if file exists
	if filePath != "" {
		if err := m.Load(); err != nil {
			return nil, fmt.Errorf("failed to load memory file: %w", err)
		}
	}

	return m, nil
}

// Load reads the memory file content
func (m *MemoryManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.filePath == "" {
		m.content = ""
		return nil
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.content = ""
			return nil
		}
		return err
	}

	m.content = string(data)
	return nil
}

// GetMemory returns the full memory content
func (m *MemoryManager) GetMemory() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.content
}

// GetMemorySection returns content from a specific markdown section
// Section name format: "section_name" extracts content under "## section_name"
func (m *MemoryManager) GetMemorySection(sectionName string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.content == "" {
		return ""
	}

	lines := strings.Split(m.content, "\n")
	sectionHeader := "## " + sectionName
	var sectionContent []string
	inSection := false

	for _, line := range lines {
		// Check if we've found the target section
		if strings.HasPrefix(line, sectionHeader) {
			inSection = true
			continue
		}

		// If we're in the section and hit another ## header, stop
		if inSection && strings.HasPrefix(line, "## ") {
			break
		}

		// If we're in the section, collect the line
		if inSection {
			sectionContent = append(sectionContent, line)
		}
	}

	return strings.TrimSpace(strings.Join(sectionContent, "\n"))
}

// AppendMemory appends content to the memory file
func (m *MemoryManager) AppendMemory(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.filePath == "" {
		return fmt.Errorf("no memory file configured")
	}

	// Ensure content ends with newline
	if !strings.HasSuffix(content, "\n") {
		content = content + "\n"
	}

	// Append separator and timestamp if content is not empty
	separator := fmt.Sprintf("\n---\n*Updated: %s*\n\n", getCurrentTimestamp())
	fullContent := separator + content

	// Append to current content
	m.content = m.content + fullContent

	// Write to file
	return os.WriteFile(m.filePath, []byte(m.content), 0644)
}

// WriteMemorySection writes or updates a specific section in the memory file
func (m *MemoryManager) WriteMemorySection(sectionName, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.filePath == "" {
		return fmt.Errorf("no memory file configured")
	}

	lines := strings.Split(m.content, "\n")
	sectionHeader := "## " + sectionName
	var newLines []string
	inSection := false
	sectionFound := false
	sectionWritten := false

	for i, line := range lines {
		// Check if we've found the target section
		if strings.HasPrefix(line, sectionHeader) {
			inSection = true
			sectionFound = true
			newLines = append(newLines, line)
			// Add timestamp
			newLines = append(newLines, fmt.Sprintf("*Updated: %s*", getCurrentTimestamp()))
			newLines = append(newLines, "")
			// Add new content
			newLines = append(newLines, content)
			sectionWritten = true
			continue
		}

		// If we're in the section and hit another ## header, stop replacing
		if inSection && strings.HasPrefix(line, "## ") {
			inSection = false
			newLines = append(newLines, "")
			newLines = append(newLines, line)
			continue
		}

		// If we're not in the target section, keep the line
		if !inSection {
			newLines = append(newLines, line)
		}

		// If we're at the end and section wasn't found, add it
		if i == len(lines)-1 && !sectionFound {
			newLines = append(newLines, "")
			newLines = append(newLines, sectionHeader)
			newLines = append(newLines, fmt.Sprintf("*Updated: %s*", getCurrentTimestamp()))
			newLines = append(newLines, "")
			newLines = append(newLines, content)
			sectionWritten = true
		}
	}

	// If content was empty and section wasn't found, create it
	if !sectionFound && m.content == "" {
		newLines = []string{
			"# Project Memory",
			"",
			sectionHeader,
			fmt.Sprintf("*Updated: %s*", getCurrentTimestamp()),
			"",
			content,
		}
		sectionWritten = true
	}

	if !sectionWritten {
		return fmt.Errorf("failed to write section %s", sectionName)
	}

	m.content = strings.Join(newLines, "\n")
	return os.WriteFile(m.filePath, []byte(m.content), 0644)
}

// HasMemory returns true if a memory file is configured
func (m *MemoryManager) HasMemory() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filePath != ""
}

// GetFilePath returns the path to the memory file
func (m *MemoryManager) GetFilePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filePath
}

// getCurrentTimestamp returns a formatted timestamp for memory updates
func getCurrentTimestamp() string {
	// Check if there's a test timestamp env var for consistent testing
	if ts := os.Getenv("COMANDA_TIMESTAMP"); ts != "" {
		return ts
	}
	// Use time package to get current time
	return time.Now().Format("2006-01-02 15:04:05")
}
