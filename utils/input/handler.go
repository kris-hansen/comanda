package input

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG format
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
)

// InputType represents the type of input being processed
type InputType int

const (
	FileInput InputType = iota
	DirectoryInput
	ScreenshotInput
	ImageInput
	WebScrapeInput
	SourceCodeInput
	StdinInput // Added StdinInput type
)

// ScrapeConfig represents the configuration for web scraping
type ScrapeConfig struct {
	URL            string            `yaml:"url"`
	AllowedDomains []string          `yaml:"allowed_domains"`
	Headers        map[string]string `yaml:"headers"`
	Extract        []string          `yaml:"extract"`
}

// Input represents a file or directory to be processed
type Input struct {
	Path         string
	Type         InputType
	Contents     []byte
	Metadata     map[string]interface{} // For additional data like scraping config
	ScrapeConfig *ScrapeConfig          // Specific configuration for web scraping
	MimeType     string                 // Added MimeType field
}

// Handler processes input files and directories
type Handler struct {
	inputs []*Input
}

// NewHandler creates a new input handler
func NewHandler() *Handler {
	return &Handler{
		inputs: make([]*Input, 0),
	}
}

// ProcessStdin handles string input as STDIN
func (h *Handler) ProcessStdin(content string) error {
	// Check if stdin is available and is a terminal/pipe
	if _, err := os.Stdin.Stat(); err != nil {
		return fmt.Errorf("error accessing stdin: %w", err)
	}

	// Validate that content is not empty
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("stdin content cannot be empty")
	}

	input := &Input{
		Path:     "STDIN",
		Type:     StdinInput,
		Contents: []byte(content),
		MimeType: "text/plain",
	}
	h.inputs = append(h.inputs, input)
	return nil
}

// getMimeType returns the appropriate MIME type for a file based on its extension
func (h *Handler) getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	// Text files
	case ".txt":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".xml":
		return "text/xml"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"

	// Documents
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	// Images
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"

	// Source code files
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".java":
		return "text/x-java"
	case ".c":
		return "text/x-c"
	case ".cpp":
		return "text/x-c++"
	case ".h":
		return "text/x-c"
	case ".hpp":
		return "text/x-c++"
	case ".rs":
		return "text/x-rust"
	case ".rb":
		return "text/x-ruby"
	case ".php":
		return "text/x-php"
	case ".swift":
		return "text/x-swift"
	case ".kt":
		return "text/x-kotlin"
	case ".scala":
		return "text/x-scala"
	case ".cs":
		return "text/x-csharp"
	case ".sh":
		return "text/x-shellscript"
	case ".pl":
		return "text/x-perl"
	case ".r":
		return "text/x-r"
	case ".sql":
		return "text/x-sql"

	default:
		return "text/plain" // Default to text/plain for unknown types
	}
}

// isSourceCode checks if the file is a source code file based on extension
func (h *Handler) isSourceCode(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	sourceExts := map[string]bool{
		".go":    true,
		".py":    true,
		".js":    true,
		".ts":    true,
		".java":  true,
		".c":     true,
		".cpp":   true,
		".h":     true,
		".hpp":   true,
		".rs":    true,
		".rb":    true,
		".php":   true,
		".swift": true,
		".kt":    true,
		".scala": true,
		".cs":    true,
		".sh":    true,
		".pl":    true,
		".r":     true,
		".sql":   true,
	}
	return sourceExts[ext]
}

// isImageFile checks if the file is an image based on extension
func (h *Handler) isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	imageExts := map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".gif":  true,
		".bmp":  true,
	}
	return imageExts[ext]
}

// ProcessPath handles both file and directory inputs
func (h *Handler) ProcessPath(path string) error {
	if path == "screenshot" {
		return h.processScreenshot()
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error accessing path %s: %w", path, err)
	}

	if fileInfo.IsDir() {
		return h.processDirectory(path)
	}

	if h.isImageFile(path) {
		return h.processImage(path)
	}

	if h.isSourceCode(path) {
		return h.processSourceCode(path)
	}

	return h.processFile(path)
}

// ProcessScrape handles web scraping input
func (h *Handler) ProcessScrape(url string, config map[string]interface{}) error {
	input := &Input{
		Path:     url,
		Type:     WebScrapeInput,
		Metadata: config,
	}

	if scrapeConfig, ok := config["scrape_config"].(map[string]interface{}); ok {
		input.ScrapeConfig = &ScrapeConfig{}
		if domains, ok := scrapeConfig["allowed_domains"].([]string); ok {
			input.ScrapeConfig.AllowedDomains = domains
		}
		if headers, ok := scrapeConfig["headers"].(map[string]string); ok {
			input.ScrapeConfig.Headers = headers
		}
		if extract, ok := scrapeConfig["extract"].([]string); ok {
			input.ScrapeConfig.Extract = extract
		}
	}

	h.inputs = append(h.inputs, input)
	return nil
}

// processFile handles single file input
func (h *Handler) processFile(path string) error {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", path, err)
	}

	input := &Input{
		Path:     path,
		Type:     FileInput,
		Contents: contents,
		MimeType: h.getMimeType(path),
	}
	h.inputs = append(h.inputs, input)
	return nil
}

// processSourceCode handles source code file input
func (h *Handler) processSourceCode(path string) error {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading source code file %s: %w", path, err)
	}

	input := &Input{
		Path:     path,
		Type:     SourceCodeInput,
		Contents: contents,
		MimeType: h.getMimeType(path),
	}
	h.inputs = append(h.inputs, input)
	return nil
}

// resizeImage resizes the image if it exceeds maximum dimensions
func (h *Handler) resizeImage(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	maxDim := 256 // Reduced from 512 to 256 to further decrease token usage

	// If image is smaller than max dimensions, return original
	if width <= maxDim && height <= maxDim {
		return img
	}

	// Calculate new dimensions while maintaining aspect ratio
	var newWidth, newHeight int
	if width > height {
		newWidth = maxDim
		newHeight = (height * maxDim) / width
	} else {
		newHeight = maxDim
		newWidth = (width * maxDim) / height
	}

	// Create new image with new dimensions
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// processImage handles image file input
func (h *Handler) processImage(path string) error {
	// Read the image file
	imgFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("error opening image %s: %w", path, err)
	}
	defer imgFile.Close()

	// Decode the image
	img, format, err := image.Decode(imgFile)
	if err != nil {
		return fmt.Errorf("error decoding image %s: %w", path, err)
	}

	// Resize image if necessary
	img = h.resizeImage(img)

	// Create a buffer to store PNG data
	var buf bytes.Buffer

	// Create PNG encoder with compression
	encoder := &png.Encoder{
		CompressionLevel: png.BestSpeed,
	}

	// Encode image to PNG with compression
	if err := encoder.Encode(&buf, img); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	// Encode PNG data to base64 with proper MIME type prefix
	mimeType := "image/png"
	if format == "jpeg" {
		mimeType = "image/jpeg"
	}
	base64Data := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(buf.Bytes()))

	input := &Input{
		Path:     path,
		Type:     ImageInput,
		Contents: []byte(base64Data),
		MimeType: mimeType,
	}
	h.inputs = append(h.inputs, input)
	return nil
}

// processDirectory handles directory input
func (h *Handler) processDirectory(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("error reading directory %s: %w", path, err)
	}

	for _, file := range files {
		fullPath := filepath.Join(path, file.Name())
		if err := h.ProcessPath(fullPath); err != nil {
			return err
		}
	}
	return nil
}

// processScreenshot captures a screenshot
func (h *Handler) processScreenshot() error {
	// Capture the primary display
	bounds := screenshot.GetDisplayBounds(0)

	// Create a new bounds with reduced resolution
	bounds.Max.X = bounds.Min.X + 512 // Reduced from 1024 to 512
	bounds.Max.Y = bounds.Min.Y + 384 // Reduced from 768 to 384

	// Capture the screen
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return fmt.Errorf("failed to capture screenshot: %w", err)
	}

	// Create a buffer to store PNG data
	var buf bytes.Buffer

	// Create PNG encoder with compression
	encoder := &png.Encoder{
		CompressionLevel: png.BestSpeed,
	}

	// Encode image to PNG with compression
	if err := encoder.Encode(&buf, img); err != nil {
		return fmt.Errorf("failed to encode screenshot: %w", err)
	}

	// Encode PNG data to base64 with proper MIME type prefix
	mimeType := "image/png"
	base64Data := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(buf.Bytes()))

	input := &Input{
		Path:     "screenshot",
		Type:     ScreenshotInput,
		Contents: []byte(base64Data),
		MimeType: mimeType,
	}
	h.inputs = append(h.inputs, input)
	return nil
}

// GetInputs returns all processed inputs
func (h *Handler) GetInputs() []*Input {
	return h.inputs
}

// GetFileContents returns the contents of a specific file
func (h *Handler) GetFileContents(path string) ([]byte, error) {
	for _, input := range h.inputs {
		if input.Path == path {
			return input.Contents, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in processed inputs", path)
}

// GetAllContents returns all file contents concatenated
func (h *Handler) GetAllContents() []byte {
	var allContents []byte
	for _, input := range h.inputs {
		if input.Type == FileInput || input.Type == ScreenshotInput || input.Type == ImageInput || input.Type == SourceCodeInput || input.Type == StdinInput {
			allContents = append(allContents, input.Contents...)
			allContents = append(allContents, '\n')
		}
	}
	return allContents
}

// Clear removes all processed inputs
func (h *Handler) Clear() {
	h.inputs = make([]*Input, 0)
}
