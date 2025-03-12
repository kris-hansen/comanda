package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/scraper"
)

// processActions handles the action section of the DSL
func (p *Processor) processActions(modelNames []string, actions []string) (string, error) {
	if len(modelNames) == 0 {
		return "", fmt.Errorf("no model specified for actions")
	}

	// For now, use the first model specified
	modelName := modelNames[0]

	// Special case: if model is NA, return the input content directly
	if modelName == "NA" {
		inputs := p.handler.GetInputs()
		if len(inputs) == 0 {
			// If there are no inputs, return empty string since there's no content to process
			return "", nil
		}

		// For NA model, concatenate all input contents
		var contents []string
		for _, inputItem := range inputs {
			contents = append(contents, string(inputItem.Contents))
		}
		return strings.Join(contents, "\n"), nil
	}

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

	for i, action := range actions {
		p.debugf("Processing action %d/%d: %s", i+1, len(actions), action)

		// Check if action is a markdown file
		if strings.HasSuffix(strings.ToLower(action), ".md") {
			content, err := fileutil.SafeReadFile(action)
			if err != nil {
				return "", fmt.Errorf("failed to read markdown file %s: %w", action, err)
			}
			action = string(content)
			p.debugf("Loaded action content from markdown file: %s", action)
		}

		inputs := p.handler.GetInputs()
		if len(inputs) == 0 {
			// If there are no inputs, just send the action directly
			return configuredProvider.SendPrompt(modelName, action)
		}

		// Process inputs based on their type
		var fileInputs []models.FileInput
		var nonFileInputs []string

		for _, inputItem := range inputs {
			switch inputItem.Type {
			case input.FileInput:
				fileInputs = append(fileInputs, models.FileInput{
					Path:     inputItem.Path,
					MimeType: inputItem.MimeType,
				})
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

				// Convert scraped data to string
				scrapedContent := fmt.Sprintf("Title: %s\n\nText Content:\n%s\n\nLinks:\n%s",
					scrapedData.Title,
					strings.Join(scrapedData.Text, "\n"),
					strings.Join(scrapedData.Links, "\n"))
				nonFileInputs = append(nonFileInputs, scrapedContent)
			default:
				nonFileInputs = append(nonFileInputs, string(inputItem.Contents))
			}
		}

		// If we have file inputs, use SendPromptWithFile
		if len(fileInputs) > 0 {
			if len(fileInputs) == 1 {
				return configuredProvider.SendPromptWithFile(modelName, action, fileInputs[0])
			}

			// Check if we should use combined or individual processing mode
			batchMode := p.getCurrentStepConfig().BatchMode
			skipErrors := p.getCurrentStepConfig().SkipErrors

			p.debugf("Multiple files detected. BatchMode=%s, SkipErrors=%v", batchMode, skipErrors)

			// If batch mode is explicitly set to "combined", use the old approach
			if batchMode == "combined" {
				p.debugf("Using combined batch mode for multiple files")
				// For multiple files, combine them into a single prompt
				var combinedPrompt string
				for i, file := range fileInputs {
					content, err := fileutil.SafeReadFile(file.Path)
					if err != nil {
						return "", fmt.Errorf("failed to read file %s: %w", file.Path, err)
					}
					combinedPrompt += fmt.Sprintf("File %d (%s):\n%s\n\n", i+1, file.Path, string(content))
				}
				combinedPrompt += fmt.Sprintf("\nAction: %s", action)
				return configuredProvider.SendPrompt(modelName, combinedPrompt)
			}

			// Default to individual processing mode (safer)
			p.debugf("Using individual processing mode for %d files", len(fileInputs))
			var results []string
			var errors []string

			for i, file := range fileInputs {
				p.debugf("Processing file %d/%d: %s", i+1, len(fileInputs), file.Path)

				// Try to process each file individually
				result, err := configuredProvider.SendPromptWithFile(modelName,
					fmt.Sprintf("For this file: %s", action), file)

				if err != nil {
					// Log error but continue with other files if skipErrors is true
					errMsg := fmt.Sprintf("Error processing file %s: %v", file.Path, err)
					p.debugf(errMsg)
					errors = append(errors, errMsg)

					// If skipErrors is false and not explicitly set, we still continue but log a warning
					if !skipErrors {
						p.debugf("Continuing despite error because individual processing mode is designed to be resilient")
					}
					continue
				}

				results = append(results, fmt.Sprintf("Results for %s:\n%s", file.Path, result))
			}

			// If all files failed, return an error
			if len(results) == 0 {
				return "", fmt.Errorf("all files failed processing: %s", strings.Join(errors, "; "))
			}

			// If some files succeeded, return their results with warnings about failed files
			combinedResult := strings.Join(results, "\n\n")
			if len(errors) > 0 {
				combinedResult += "\n\nWarning: Some files could not be processed:\n" +
					strings.Join(errors, "\n")
			}

			return combinedResult, nil
		}

		// If we have non-file inputs, combine them and use SendPrompt
		if len(nonFileInputs) > 0 {
			combinedInput := strings.Join(nonFileInputs, "\n\n")
			return configuredProvider.SendPrompt(modelName, fmt.Sprintf("Input:\n%s\n\nAction: %s", combinedInput, action))
		}
	}

	return "", fmt.Errorf("no actions processed")
}
