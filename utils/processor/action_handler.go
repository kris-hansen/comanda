package processor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/scraper"
)

// getMimeType returns the MIME type for a file based on its extension
func (p *Processor) getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}

// processActions handles the action section of the DSL
func (p *Processor) processActions(modelNames []string, actions []string) (string, error) {
	if len(modelNames) == 0 {
		return "", fmt.Errorf("no model specified for actions")
	}

	// For now, use the first model specified
	modelName := modelNames[0]

	// Get provider by detecting it from the model name
	provider := models.DetectProvider(modelName)
	if provider == nil {
		return "", fmt.Errorf("provider not found for model: %s", modelName)
	}

	// Use the configured provider instance
	configuredProvider := p.providers[provider.Name()]
	if configuredProvider == nil {
		return "", fmt.Errorf("provider %s not configured", provider.Name())
	}

	p.debugf("Using model %s with provider %s", modelName, configuredProvider.Name())
	p.debugf("Processing %d action(s)", len(actions))

	var finalResponse string

	for i, action := range actions {
		p.debugf("Processing action %d/%d: %s", i+1, len(actions), action)

		// Handle each input
		for _, inputItem := range p.handler.GetInputs() {
			var response string
			var err error

			switch inputItem.Type {
			case input.WebScrapeInput:
				// Handle scraping input
				scraper := scraper.NewScraper()
				if config, ok := inputItem.Metadata["scrape_config"].(map[string]interface{}); ok {
					if domains, ok := config["allowed_domains"].([]interface{}); ok {
						allowedDomains := make([]string, len(domains))
						for i, d := range domains {
							allowedDomains[i] = d.(string)
						}
						scraper.AllowedDomains(allowedDomains...)
					}
					if headers, ok := config["headers"].(map[string]interface{}); ok {
						headerMap := make(map[string]string)
						for k, v := range headers {
							headerMap[k] = v.(string)
						}
						scraper.SetCustomHeaders(headerMap)
					}
				}
				scrapedData, err := scraper.Scrape(inputItem.Path)
				if err != nil {
					return "", fmt.Errorf("failed to scrape URL %s: %w", inputItem.Path, err)
				}

				// Convert scraped data to string for model processing
				scrapedContent := fmt.Sprintf("Title: %s\n\nText Content:\n%s\n\nLinks:\n%s",
					scrapedData.Title,
					strings.Join(scrapedData.Text, "\n"),
					strings.Join(scrapedData.Links, "\n"))

				response, err = configuredProvider.SendPrompt(modelName, fmt.Sprintf("Scraped Content:\n%s\n\nAction: %s", scrapedContent, action))
			case input.FileInput:
				if p.validator.IsDocumentFile(inputItem.Path) || strings.HasSuffix(inputItem.Path, ".csv") {
					fileInput := models.FileInput{
						Path:     inputItem.Path,
						MimeType: p.getMimeType(inputItem.Path),
					}
					response, err = configuredProvider.SendPromptWithFile(modelName, action, fileInput)
				} else {
					fullPrompt := fmt.Sprintf("Input:\n%s\nAction: %s", string(inputItem.Contents), action)
					response, err = configuredProvider.SendPrompt(modelName, fullPrompt)
				}
			default:
				fullPrompt := fmt.Sprintf("Input:\n%s\nAction: %s", string(inputItem.Contents), action)
				response, err = configuredProvider.SendPrompt(modelName, fullPrompt)
			}

			if err != nil {
				return "", fmt.Errorf("failed to process input %s with model %s: %w", inputItem.Path, modelName, err)
			}
			finalResponse = response
		}
	}

	return finalResponse, nil
}
