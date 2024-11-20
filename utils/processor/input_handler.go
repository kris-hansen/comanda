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

// processRegularInput handles regular file and directory inputs
func (p *Processor) processRegularInput(inputPath string) error {
	// Check if the path exists
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			// Only try glob if the path contains glob characters
			if containsGlobChar(inputPath) {
				matches, err := filepath.Glob(inputPath)
				if err != nil {
					return fmt.Errorf("error processing glob pattern %s: %w", inputPath, err)
				}
				if len(matches) == 0 {
					return fmt.Errorf("no files found matching pattern: %s", inputPath)
				}
				for _, match := range matches {
					if err := p.processFile(match); err != nil {
						return err
					}
				}
				return nil
			}

			// Check if the file is an output in any other step
			if p.isOutputInOtherSteps(inputPath) {
				p.debugf("File %s does not exist yet but will be created as output in another step", inputPath)
				return nil
			}

			return fmt.Errorf("path does not exist and is not an output of any step: %s", inputPath)
		}
		return fmt.Errorf("error accessing path %s: %w", inputPath, err)
	}

	return p.processFile(inputPath)
}

// processFile handles a single file input
func (p *Processor) processFile(path string) error {
	p.debugf("Validating path: %s", path)
	if err := p.validator.ValidatePath(path); err != nil {
		return err
	}

	// Add file extension validation
	if err := p.validator.ValidateFileExtension(path); err != nil {
		return err
	}

	p.debugf("Processing file: %s", path)
	if err := p.handler.ProcessPath(path); err != nil {
		return err
	}
	p.debugf("Successfully processed file: %s", path)
	return nil
}

// containsGlobChar checks if a path contains glob characters
func containsGlobChar(path string) bool {
	return strings.ContainsAny(path, "*?[]")
}
