package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/scraper"
)

// PromptPrefix is used to instruct providers to output only the requested content without metadata.
const PromptPrefix = "IMPORTANT: Provide ONLY the requested output content without any headers, labels, file annotations, or metadata. Do not include phrases like 'Results for' or 'File:' or any other wrapper text. Output the pure content as requested.\n\n"

// ActionResult holds the results from processing actions
// It can contain either a single combined result or multiple individual results (for chunking)
type ActionResult struct {
	// Single combined result (used when not chunking or when batch_mode is "combined")
	CombinedResult string

	// Individual results (used when chunking with batch_mode "individual")
	IndividualResults []string

	// Corresponding input paths for each individual result (for chunk identification)
	InputPaths []string

	// Whether this contains individual results
	HasIndividualResults bool
}

// processActions handles the action section of the DSL
// Returns ActionResult which may contain either combined or individual results
func (p *Processor) processActions(modelNames []string, actions []string) (*ActionResult, error) {
	if len(modelNames) == 0 {
		return nil, fmt.Errorf("no model specified for actions")
	}

	// For now, use the first model specified
	modelName := modelNames[0]

	// Special case: if model is NA, return the input content directly
	if modelName == "NA" {
		inputs := p.handler.GetInputs()
		if len(inputs) == 0 {
			// If there are no inputs, return empty string since there's no content to process
			return &ActionResult{CombinedResult: "", HasIndividualResults: false}, nil
		}

		// For NA model, concatenate all input contents
		var contents []string
		for _, inputItem := range inputs {
			contents = append(contents, string(inputItem.Contents))
		}
		return &ActionResult{
			CombinedResult:       strings.Join(contents, "\n"),
			HasIndividualResults: false,
		}, nil
	}

	// Get provider by detecting it from the model name
	provider := models.DetectProvider(modelName)
	if provider == nil {
		return nil, fmt.Errorf("provider not found for model: %s", modelName)
	}

	// Use the configured provider instance
	configuredProvider := p.providers[provider.Name()]
	if configuredProvider == nil {
		return nil, fmt.Errorf("provider %s not configured", provider.Name())
	}

	p.debugf("Using model %s with provider %s", modelName, configuredProvider.Name())
	p.debugf("Processing %d action(s)", len(actions))

	// Check if we're in agentic mode (have allowed paths set in agentic loop)
	agenticConfig := p.getAgenticConfig()
	isAgenticMode := agenticConfig != nil && len(agenticConfig.AllowedPaths) > 0

	for i, action := range actions {
		p.debugf("Processing action %d/%d: %s", i+1, len(actions), action)

		// Check if action is a markdown file
		if strings.HasSuffix(strings.ToLower(action), ".md") {
			content, err := fileutil.SafeReadFile(action)
			if err != nil {
				return nil, fmt.Errorf("failed to read markdown file %s: %w", action, err)
			}
			action = string(content)
			p.debugf("Loaded action content from markdown file: %s", action)
		}

		inputs := p.handler.GetInputs()
		if len(inputs) == 0 {
			// If there are no inputs, just send the action directly
			// Check for agentic mode with Claude Code
			if isAgenticMode {
				if claudeCode, ok := configuredProvider.(*models.ClaudeCodeProvider); ok {
					p.debugf("Using agentic mode with Claude Code (paths: %v, tools: %v)",
						agenticConfig.AllowedPaths, agenticConfig.Tools)
					// Pass stream log path to claude-code for debug visibility
					streamLogPath := p.GetStreamLogPath()
					p.debugf("Stream log path: %q", streamLogPath)
					var debugWatcher *DebugWatcher
					if streamLogPath != "" {
						debugPath := streamLogPath + ".claude-debug"
						p.debugf("Setting claude-code debug file: %s", debugPath)
						claudeCode.SetDebugFile(debugPath)
						// Start watching the debug file for context usage
						if p.streamLog != nil {
							debugWatcher = NewDebugWatcher(debugPath, p.streamLog)
							debugWatcher.Start()
						}
					}
					result, err := claudeCode.SendPromptAgentic(modelName, action,
						agenticConfig.AllowedPaths, agenticConfig.Tools, p.getEffectiveWorkDir())
					// Stop the debug watcher
					if debugWatcher != nil {
						debugWatcher.Stop()
					}
					if err != nil {
						return nil, err
					}
					return &ActionResult{
						CombinedResult:       result,
						HasIndividualResults: false,
					}, nil
				}
			}
			result, err := configuredProvider.SendPrompt(modelName, action)
			if err != nil {
				return nil, err
			}
			return &ActionResult{
				CombinedResult:       result,
				HasIndividualResults: false,
			}, nil
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
					return nil, fmt.Errorf("failed to scrape URL %s: %w", inputItem.Path, err)
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
				result, err := configuredProvider.SendPromptWithFile(modelName, action, fileInputs[0])
				if err != nil {
					return nil, err
				}
				return &ActionResult{
					CombinedResult:       result,
					HasIndividualResults: false,
				}, nil
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
						return nil, fmt.Errorf("failed to read file %s: %w", file.Path, err)
					}
					combinedPrompt += fmt.Sprintf("File %d (%s):\n%s\n\n", i+1, file.Path, string(content))
				}
				combinedPrompt += fmt.Sprintf("\nAction: %s", action)
				result, err := configuredProvider.SendPrompt(modelName, combinedPrompt)
				if err != nil {
					return nil, err
				}
				return &ActionResult{
					CombinedResult:       result,
					HasIndividualResults: false,
				}, nil
			}

			// Default to individual processing mode (safer)
			// This is KEY for chunking support - we keep individual results separate
			p.debugf("Using individual processing mode for %d files", len(fileInputs))
			var results []string
			var inputPaths []string
			var errors []string

			for i, file := range fileInputs {
				p.debugf("Processing file %d/%d: %s", i+1, len(fileInputs), file.Path)

				// Build a clean prompt that discourages metadata wrapping
				// Detect output format from action to provide appropriate instructions
				// Try to process each file individually
				result, err := configuredProvider.SendPromptWithFile(modelName,
					fmt.Sprintf("%sFor this file: %s", PromptPrefix, action), file)

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

				// Store the result and its corresponding input path
				results = append(results, result)
				inputPaths = append(inputPaths, file.Path)
			}

			// If all files failed, return an error
			if len(results) == 0 {
				return nil, fmt.Errorf("all files failed processing: %s", strings.Join(errors, "; "))
			}

			// Return individual results for chunking support
			// The caller will decide whether to combine them or write them separately
			return &ActionResult{
				IndividualResults:    results,
				InputPaths:           inputPaths,
				HasIndividualResults: true,
			}, nil
		}

		// If we have non-file inputs, combine them and use SendPrompt
		if len(nonFileInputs) > 0 {
			combinedInput := strings.Join(nonFileInputs, "\n\n")
			combinedPrompt := fmt.Sprintf("Input:\n%s\n\nAction: %s", combinedInput, action)

			// Check for agentic mode with Claude Code
			if isAgenticMode {
				if claudeCode, ok := configuredProvider.(*models.ClaudeCodeProvider); ok {
					p.debugf("Using agentic mode with Claude Code for non-file inputs")
					// Pass stream log path to claude-code for debug visibility
					streamLogPath := p.GetStreamLogPath()
					p.debugf("Stream log path (non-file): %q", streamLogPath)
					var debugWatcher *DebugWatcher
					if streamLogPath != "" {
						debugPath := streamLogPath + ".claude-debug"
						p.debugf("Setting claude-code debug file: %s", debugPath)
						claudeCode.SetDebugFile(debugPath)
						// Start watching the debug file for context usage
						if p.streamLog != nil {
							debugWatcher = NewDebugWatcher(debugPath, p.streamLog)
							debugWatcher.Start()
						}
					}
					result, err := claudeCode.SendPromptAgentic(modelName, combinedPrompt,
						agenticConfig.AllowedPaths, agenticConfig.Tools, p.getEffectiveWorkDir())
					// Stop the debug watcher
					if debugWatcher != nil {
						debugWatcher.Stop()
					}
					if err != nil {
						return nil, err
					}
					return &ActionResult{
						CombinedResult:       result,
						HasIndividualResults: false,
					}, nil
				}
			}

			result, err := configuredProvider.SendPrompt(modelName, combinedPrompt)
			if err != nil {
				return nil, err
			}
			return &ActionResult{
				CombinedResult:       result,
				HasIndividualResults: false,
			}, nil
		}
	}

	return nil, fmt.Errorf("no actions processed")
}
