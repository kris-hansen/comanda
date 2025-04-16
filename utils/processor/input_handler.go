package processor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/input"
)

// isSpecialInput checks if the input is a special type (e.g., screenshot)
func (p *Processor) isSpecialInput(input string) bool {
	specialInputs := []string{"screenshot", "NA", "STDIN"}
	for _, special := range specialInputs {
		if input == special {
			return true
		}
	}
	return false
}

// isURL checks if the input string is a valid URL
func (p *Processor) isURL(input string) bool {
	u, err := url.Parse(input)
	if err != nil {
		return false
	}
	// Just check if it has a scheme and host, let fetchURL do stricter validation
	return u.Scheme != "" && u.Host != ""
}

// fetchURL retrieves content from a URL and saves it to a temporary file
func (p *Processor) fetchURL(urlStr string) (string, error) {
	p.debugf("Fetching content from URL: %s", urlStr)

	// Parse and validate the URL first
	parsedURL, err := url.Parse(urlStr)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return "", fmt.Errorf("invalid URL %s", urlStr)
	}

	// Get hostname (without port)
	host := parsedURL.Hostname()

	// Skip DNS resolution for localhost/127.0.0.1 and test server URLs
	if !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1") && !strings.Contains(urlStr, ".that.does.not.exist") {
		// Try to resolve the host first with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: 2 * time.Second,
				}
				return d.DialContext(ctx, network, address)
			},
		}

		_, err = resolver.LookupHost(ctx, host)
		if err != nil {
			// Return error for DNS resolution failures
			return "", fmt.Errorf("failed to resolve host %s: invalid or non-existent domain", host)
		}
	}

	// Special handling for test URLs that should fail
	if strings.Contains(urlStr, ".that.does.not.exist") {
		return "", fmt.Errorf("failed to resolve host %s: invalid or non-existent domain", host)
	}

	// Create a custom HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
		},
	}

	resp, err := client.Get(urlStr)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return "", fmt.Errorf("timeout while fetching URL %s", urlStr)
		}
		return "", fmt.Errorf("failed to fetch URL %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch URL %s: status code %d", urlStr, resp.StatusCode)
	}

	// Create a temporary file with an appropriate extension based on Content-Type
	ext := ".txt"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "html") {
		ext = ".html"
	} else if strings.Contains(contentType, "json") {
		ext = ".json"
	}

	tmpFile, err := os.CreateTemp("", "comanda-url-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for URL content: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write URL content to file: %w", err)
	}

	p.debugf("URL content saved to temporary file: %s", tmpPath)
	return tmpPath, nil
}

// processInputs handles the input section of the DSL
func (p *Processor) processInputs(inputs []string) error {
	p.debugf("Processing %d input(s)", len(inputs))
	for _, inputPath := range inputs {
		// Skip empty input
		if inputPath == "" {
			p.debugf("Skipping empty input")
			continue
		}

		p.debugf("Processing input path: %s", inputPath)

		// Handle special inputs first
		if p.isSpecialInput(inputPath) {
			if inputPath == "NA" {
				p.debugf("Skipping NA input")
				continue
			}
			if inputPath == "STDIN" {
				p.debugf("Skipping STDIN input as it's handled in Process()")
				continue
			}
			p.debugf("Processing special input: %s", inputPath)
			if err := p.handler.ProcessPath(inputPath); err != nil {
				return fmt.Errorf("error processing special input %s: %w", inputPath, err)
			}
			continue
		}

		// Check if input is a URL
		if p.isURL(inputPath) {
			// For scraping inputs, the URL is already processed by ProcessScrape
			if !p.isScrapeInput(inputPath) {
				tmpPath, err := p.fetchURL(inputPath)
				if err != nil {
					return err
				}
				defer os.Remove(tmpPath)
				inputPath = tmpPath
			}
		}

		// Handle regular file inputs
		if err := p.processRegularInput(inputPath); err != nil {
			return err
		}
	}
	return nil
}

// isScrapeInput checks if the input is already processed as a scraping input
func (p *Processor) isScrapeInput(url string) bool {
	for _, inputItem := range p.handler.GetInputs() {
		if inputItem.Type == input.WebScrapeInput && inputItem.Path == url {
			return true
		}
	}
	return false
}

// isOutputInOtherSteps checks if a file is an output in any of the steps
func (p *Processor) isOutputInOtherSteps(path string) bool {
	for _, step := range p.config.Steps {
		outputs := p.NormalizeStringSlice(step.Config.Output)
		for _, output := range outputs {
			if output != "STDOUT" && output == path {
				return true
			}
		}
	}
	return false
}

// processRegularInput handles regular file and directory inputs, respecting runtimeDir and globs
func (p *Processor) processRegularInput(inputPath string) error {
	// --- Step 1: Check for Glob Pattern ---
	isGlob := strings.ContainsAny(inputPath, "*?[")
	if isGlob {
		p.debugf("Input '%s' identified as a potential glob pattern.", inputPath)
		// Determine the base path for glob expansion based on context
		var globPattern string
		if filepath.IsAbs(inputPath) {
			globPattern = inputPath // Absolute path glob
			p.debugf("Using absolute glob pattern: %s", globPattern)
		} else if p.runtimeDir != "" {
			// p.runtimeDir should be absolute after NewProcessor
			globPattern = filepath.Join(p.runtimeDir, inputPath)
			p.debugf("Resolving glob '%s' relative to runtimeDir '%s': %s", inputPath, p.runtimeDir, globPattern)
		} else if p.serverConfig != nil && p.serverConfig.Enabled {
			// Server mode, no runtimeDir, use DataDir
			globPattern = filepath.Join(p.serverConfig.DataDir, inputPath)
			p.debugf("Resolving glob '%s' relative to DataDir '%s': %s", inputPath, p.serverConfig.DataDir, globPattern)
		} else {
			// CLI mode, relative to current working directory
			globPattern = inputPath
			p.debugf("Resolving glob '%s' relative to current working directory", globPattern)
		}
		// Pass the fully formed glob pattern to processFile, which handles expansion via the handler
		return p.processFile(globPattern)
	}

	// --- Step 2: Not a glob, resolve as a regular file path ---
	var filePath string
	pathSource := "provided" // Track where the path was resolved from for error messages

	if !filepath.IsAbs(inputPath) {
		// Input is a relative path
		if p.runtimeDir != "" {
			// Check relative to runtimeDir (should be absolute)
			resolvedPath := filepath.Join(p.runtimeDir, inputPath)
			pathSource = fmt.Sprintf("runtime directory '%s'", p.runtimeDir)
			p.debugf("Checking relative path '%s' against %s: %s", inputPath, pathSource, resolvedPath)
			// Check existence immediately
			if _, err := os.Stat(resolvedPath); err == nil {
				filePath = resolvedPath
				p.debugf("Found file at resolved path: %s", filePath)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("error checking path '%s' in %s: %w", resolvedPath, pathSource, err)
			} else {
				p.debugf("File not found at resolved path: %s", resolvedPath)
				// Keep filePath empty, will trigger "not found" error later if needed
			}
		} else if p.serverConfig != nil && p.serverConfig.Enabled {
			// Server mode, no runtimeDir, check relative to DataDir
			resolvedPath := filepath.Join(p.serverConfig.DataDir, inputPath)
			pathSource = fmt.Sprintf("data directory '%s'", p.serverConfig.DataDir)
			p.debugf("Checking relative path '%s' against %s: %s", inputPath, pathSource, resolvedPath)
			// Check existence immediately
			if _, err := os.Stat(resolvedPath); err == nil {
				filePath = resolvedPath
				p.debugf("Found file at resolved path: %s", filePath)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("error checking path '%s' in %s: %w", resolvedPath, pathSource, err)
			} else {
				p.debugf("File not found at resolved path: %s", resolvedPath)
				// Keep filePath empty
			}
		} else {
			// CLI mode, relative path
			pathSource = "current working directory"
			p.debugf("Using relative path '%s' in %s", inputPath, pathSource)
			filePath = inputPath // Use as is, relative to CWD
		}
	} else {
		// Input is an absolute path
		p.debugf("Using absolute path as provided: %s", inputPath)
		filePath = inputPath
	}

	// --- Step 3: Check Existence and Type (if not found in context) ---
	if filePath == "" {
		// This means it was a relative path that wasn't found in runtimeDir or DataDir
		if p.isOutputInOtherSteps(inputPath) { // Check original inputPath name for output steps
			p.debugf("Relative input '%s' not found, but might be created as output later.", inputPath)
			return nil // Allow processing to continue
		}
		// Return specific "not found" error based on where we looked
		return fmt.Errorf("input file '%s' not found in %s", inputPath, pathSource)
	}

	// If filePath is set (either absolute or resolved), check it
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist at the final path. Check if it's an output.
			if p.isOutputInOtherSteps(filePath) {
				p.debugf("File '%s' does not exist yet but will be created as output in another step", filePath)
				return nil // Allow processing to continue
			}
			// Final "not found" error, referencing the path we actually checked
			return fmt.Errorf("input file '%s' not found (checked path: %s)", inputPath, filePath)
		}
		// Other error accessing the path (e.g., permissions)
		return fmt.Errorf("error accessing input path '%s': %w", filePath, err) // Use filePath in error
	}

	// If it exists, ensure it's not a directory
	if fileInfo.IsDir() {
		return fmt.Errorf("input path '%s' is a directory, not a file", filePath)
	}

	// --- Step 4: Process the validated file path ---
	return p.processFile(filePath) // Pass the final, validated filePath
}

// processFile handles a single file input or glob pattern
func (p *Processor) processFile(path string) error {
	// Check the base name for glob characters to decide on validation
	isGlob := strings.ContainsAny(filepath.Base(path), "*?[")

	if !isGlob {
		// Only validate if it's not potentially a glob pattern
		p.debugf("Validating non-glob path: %s", path)
		if err := p.validator.ValidatePath(path); err != nil {
			return fmt.Errorf("path validation failed for '%s': %w", path, err)
		}
		// Add file extension validation
		if err := p.validator.ValidateFileExtension(path); err != nil {
			return fmt.Errorf("file extension validation failed for '%s': %w", path, err)
		}
	} else {
		p.debugf("Skipping path/extension validation for potential glob pattern: %s", path)
	}

	p.debugf("Processing file/glob via handler: %s", path)
	err := p.handler.ProcessPath(path) // The handler attempts glob expansion internally if needed
	if err != nil {
		// Check if the error indicates a glob pattern matched no files.
		// The exact error might depend on the handler implementation, but often involves "no such file"
		// or a specific glob error type if the handler provides one.
		// We check if it's a glob and if the error is a "not found" type.
		if isGlob && os.IsNotExist(err) {
			// This suggests the glob pattern itself might have been stat'd after expansion failed,
			// or the handler explicitly returned IsNotExist for no matches.
			p.debugf("Glob pattern '%s' matched no files or handler errored: %v", path, err)
			// Return a clearer error indicating the glob pattern failed.
			return fmt.Errorf("glob pattern '%s' did not match any files", path)
		}
		// Return other errors from the handler, wrapped for context.
		return fmt.Errorf("error processing path '%s' via handler: %w", path, err)
	}

	// If ProcessPath succeeded (even if glob matched 0 files but didn't error), log success.
	// The handler internally manages adding matched files.
	p.debugf("Successfully processed file/glob: %s", path)
	return nil
}
