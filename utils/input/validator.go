package input

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
)

// Common file extensions
var (
	TextExtensions = []string{
		".txt",
		".md",
		".yml",
		".yaml",
		".html", // Added for URL content
		".json", // Added for URL content
		".csv",  // Added for CSV support
		".xml",  // Added XML support
	}

	ImageExtensions = []string{
		".png",
		".jpg",
		".jpeg",
		".gif",
		".bmp",
	}

	DocumentExtensions = []string{
		".pdf",
		".doc",
		".docx",
	}

	SourceCodeExtensions = []string{
		".go",
		".py",
		".js",
		".ts",
		".java",
		".c",
		".cpp",
		".h",
		".hpp",
		".rs",
		".rb",
		".php",
		".swift",
		".kt",
		".scala",
		".cs",
		".sh",
		".pl",
		".r",
		".sql",
	}
)

// Validator validates input paths
type Validator struct {
	allowedExtensions []string
}

// NewValidator creates a new input validator with default text extensions
func NewValidator(additionalExtensions []string) *Validator {
	// Start with text, image, document, and source code extensions
	allExtensions := append([]string{}, TextExtensions...)
	allExtensions = append(allExtensions, ImageExtensions...)
	allExtensions = append(allExtensions, DocumentExtensions...)
	allExtensions = append(allExtensions, SourceCodeExtensions...)

	// Add any additional extensions
	if len(additionalExtensions) > 0 {
		allExtensions = append(allExtensions, additionalExtensions...)
	}

	// Debug print all allowed extensions
	config.DebugLog("[Validator] Allowed extensions: %v", allExtensions)

	return &Validator{
		allowedExtensions: allExtensions,
	}
}

// ValidatePath checks if the path is valid
func (v *Validator) ValidatePath(path string) error {
	// Special case for screenshot input
	if path == "screenshot" {
		return nil
	}

	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	return nil
}

// ValidateFileExtension checks if the file has an allowed extension
func (v *Validator) ValidateFileExtension(path string) error {
	// Special case for screenshot input
	if path == "screenshot" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return fmt.Errorf("file must have an extension")
	}

	config.DebugLog("[Validator] Checking extension %s against allowed extensions: %v", ext, v.allowedExtensions)

	for _, allowedExt := range v.allowedExtensions {
		if ext == allowedExt {
			config.DebugLog("[Validator] Found matching extension: %s", ext)
			return nil
		}
	}

	return fmt.Errorf("file extension %s is not allowed", ext)
}

// IsImageFile checks if the file has an image extension
func (v *Validator) IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, imgExt := range ImageExtensions {
		if ext == imgExt {
			return true
		}
	}
	return false
}

// IsDocumentFile checks if the file has a document extension
func (v *Validator) IsDocumentFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	config.DebugLog("[Validator] Checking if %s is a document extension", ext)
	for _, docExt := range DocumentExtensions {
		if ext == docExt {
			config.DebugLog("[Validator] Found matching document extension: %s", ext)
			return true
		}
	}
	return false
}

// IsSourceCodeFile checks if the file has a source code extension
func (v *Validator) IsSourceCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	config.DebugLog("[Validator] Checking if %s is a source code extension", ext)
	for _, srcExt := range SourceCodeExtensions {
		if ext == srcExt {
			config.DebugLog("[Validator] Found matching source code extension: %s", ext)
			return true
		}
	}
	return false
}
