package input

import (
	"fmt"
	"path/filepath"
	"strings"
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
	}

	ImageExtensions = []string{
		".png",
		".jpg",
		".jpeg",
		".gif",
		".bmp",
	}
)

// Validator validates input paths
type Validator struct {
	allowedExtensions []string
}

// NewValidator creates a new input validator with default text extensions
func NewValidator(additionalExtensions []string) *Validator {
	// Start with text and image extensions
	allExtensions := append([]string{}, TextExtensions...)
	allExtensions = append(allExtensions, ImageExtensions...)

	// Add any additional extensions
	if len(additionalExtensions) > 0 {
		allExtensions = append(allExtensions, additionalExtensions...)
	}

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

	for _, allowedExt := range v.allowedExtensions {
		if ext == allowedExt {
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
