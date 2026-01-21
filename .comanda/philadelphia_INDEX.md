# philadelphia - Codebase Index

**Languages:** go
**Files:** 74 total, 74 indexed

This appears to be a CLI tool.

## Repository Layout

```
philadelphia/
  Makefile
  go.mod
  go.sum
  main.go
  .comanda/
  .context/
    attachments/
    plans/
  .github/
    workflows/
  .rules/
  cmd/
    chart.go
    configure.go
    process.go
    root.go
    server.go
  docs/
  examples/
    agentic-loop/
    claude-code/
    codebase-index/
      sample-project/
        go.mod
    database-connections/
    defer-example/
    document-processing/
    file-processing/
    gemini-cli/
    image-processing/
    model-examples/
    multi-agent/
    openai-codex/
    openai-compat/
    parallel-processing/
    responses-api/
    server-examples/
    tool-use/
    web-scraping/
  tests/
    integration/
  utils/
    chunker/
      chunker.go
    codebaseindex/
      adapter.go
      extract.go
      manager.go
      scan.go
      store.go
      synthesize.go
      types.go
      adapters/
    config/
      env.go
      memory.go
      server.go
    database/
    discovery/
      discovery.go
    fileutil/
      fileutil.go
    input/
      handler.go
      validator.go
    models/
      anthropic.go
      claudecode.go
      deepseek.go
      geminicli.go
      google.go
      moonshot.go
      ollama.go
      openai.go
      openaicodex.go
      provider.go
      registry.go
      ... and 2 more files
    processor/
      action_handler.go
      agentic_loop.go
      codebase_index_handler.go
      database_handler.go
      dsl.go
      embedded_guide.go
      input_handler.go
      memory.go
      model_handler.go
      output_handler.go
      progress.go
      ... and 5 more files
    retry/
      retry.go
    scraper/
      scraper.go
    server/
      auth.go
      bulk_operations.go
      env_handlers.go
      file_backup.go
      file_handlers.go
      generate_handler.go
      handlers.go
      logging.go
      openai_handlers.go
      openai_types.go
      provider_handlers.go
      ... and 3 more files
```

## Primary Capabilities

- **cmd**: CLI commands, entrypoints
- **docs**: documentation
- **tests**: testing
- **utils**: utilities, helpers

## Entry Points

- `main.go` (package: main)
- `examples/codebase-index/sample-project/cmd/server/main.go` (package: main)

## Key Modules

### cmd

Files:
- `cmd/chart.go`
- `cmd/server.go`
- `cmd/process.go`
- `cmd/root.go`
- `cmd/configure.go`

### examples

Files:
- `examples/codebase-index/sample-project/cmd/server/main.go`
- `examples/codebase-index/sample-project/go.mod`
- `examples/codebase-index/sample-project/pkg/handlers/users.go`
- `examples/codebase-index/sample-project/internal/config/config.go`
- `examples/codebase-index/sample-project/pkg/models/user.go`

### utils

Files:
- `utils/processor/action_handler.go`
- `utils/processor/progress.go`
- `utils/codebaseindex/types.go`
- `utils/config/env.go`
- `utils/config/memory.go`
- `utils/config/server.go`
- `utils/discovery/discovery.go`
- `utils/fileutil/fileutil.go`
- `utils/input/handler.go`
- `utils/input/validator.go`
- `utils/models/anthropic.go`
- `utils/models/claudecode.go`
- `utils/models/deepseek.go`
- `utils/models/geminicli.go`
- `utils/models/google.go`
- `utils/models/moonshot.go`
- `utils/models/ollama.go`
- `utils/models/openai.go`
- `utils/models/openaicodex.go`
- `utils/models/provider.go`
- `utils/codebaseindex/synthesize.go`
- `utils/codebaseindex/store.go`
- `utils/codebaseindex/scan.go`
- `utils/chunker/chunker.go`
- `utils/codebaseindex/adapter.go`
- `utils/codebaseindex/extract.go`
- `utils/codebaseindex/manager.go`
- `utils/processor/dsl.go`
- `utils/processor/input_handler.go`
- `utils/processor/embedded_guide.go`
- `utils/processor/memory.go`
- `utils/processor/output_handler.go`
- `utils/processor/database_handler.go`
- `utils/processor/model_handler.go`
- `utils/processor/responses_handler.go`
- `utils/processor/codebase_index_handler.go`
- `utils/processor/tool_executor.go`
- `utils/processor/types.go`
- `utils/retry/retry.go`
- `utils/processor/utils.go`
- `utils/processor/spinner.go`
- `utils/scraper/scraper.go`
- `utils/server/env_handlers.go`
- `utils/processor/agentic_loop.go`
- `utils/server/file_backup.go`
- `utils/server/file_handlers.go`
- `utils/server/generate_handler.go`
- `utils/server/handlers.go`
- `utils/server/logging.go`
- `utils/server/openai_handlers.go`
- `utils/server/openai_types.go`
- `utils/server/provider_handlers.go`
- `utils/server/server.go`
- `utils/server/types.go`
- `utils/server/bulk_operations.go`
- `utils/server/utils.go`
- `utils/models/xai.go`
- `utils/models/registry.go`
- `utils/server/auth.go`
- `utils/models/vllm.go`

## Important Files

### `main.go`

**Package:** main

**Functions:** `main`

### `cmd/chart.go`

**Package:** cmd

**Types:** `ChartNode` (struct), `WorkflowChart` (struct)

**Functions:** `init`, `buildWorkflowChart`, `stepToChartNode`, `validateNode`, `buildDependencies`, `renderChart`, `printBox`, `printSmallBox`, ... +12 more

**Frameworks:** cobra

### `cmd/server.go`

**Package:** cmd

**Functions:** `configureServer`, `configureCORS`, `init`

**Frameworks:** cobra

### `cmd/process.go`

**Package:** cmd

**Functions:** `init`, `parseVarsFlags`

**Frameworks:** cobra

### `cmd/root.go`

**Package:** cmd

**Functions:** `buildGeneratePrompt`, `extractYAMLContent`, `init`, `getVersion`, `Execute`

**Frameworks:** cobra

### `cmd/configure.go`

**Package:** cmd

**Types:** `OllamaModel` (struct), `VLLMModel` (struct)

**Functions:** `isUnsupportedModel`, `isPrimaryOpenAIModel`, `getOpenAIModelsAndCategorize`, `getOpenAIModels`, `getAnthropicModelsAndCategorize`, `getAnthropicModels`, `getXAIModels`, `getDeepseekModels`, ... +26 more

### `go.mod`

### `Makefile`

### `go.sum`

### `utils/processor/action_handler.go`

**Package:** processor

**Types:** `ActionResult` (struct)

**Functions:** `processActions`

### `utils/processor/progress.go`

**Package:** processor

**Types:** `ProgressType` (type), `StepInfo` (struct), `ProgressUpdate` (struct), `ProgressWriter` (interface), `channelProgressWriter` (struct)

**Functions:** `NewChannelProgressWriter`, `WriteProgress`

### `utils/codebaseindex/types.go`

**Package:** codebaseindex

**Types:** `HashAlgorithm` (type), `StoreLocation` (type), `Config` (struct), `AdapterOverride` (struct), `Result` (struct), `ScanResult` (struct), `FileEntry` (struct), `DirNode` (struct), ... +4 more

**Functions:** `DefaultConfig`

### `utils/config/env.go`

**Package:** config

**Types:** `ModelMode` (type), `DatabaseType` (type), `DatabaseConfig` (struct), `Model` (struct), `Provider` (struct), `ToolConfig` (struct), `EnvConfig` (struct)

**Functions:** `DebugLog`, `VerboseLog`, `GetComandaDir`, `EnsureComandaDir`, `GetEnvPath`, `PromptPassword`, `deriveKey`, `IsEncrypted`, ... +24 more

### `utils/config/memory.go`

**Package:** config

**Functions:** `GetMemoryPath`, `fileExists`, `InitializeUserMemoryFile`

### `utils/config/server.go`

**Package:** config

**Types:** `ServerConfig` (struct), `CORS` (struct), `OpenAICompatConfig` (struct)

### `utils/discovery/discovery.go`

**Package:** discovery

**Types:** `OllamaModel` (struct), `VLLMModel` (struct)

**Functions:** `GetOpenAIModels`, `GetAnthropicModels`, `GetXAIModels`, `GetDeepseekModels`, `GetGoogleModels`, `CheckOllamaInstalled`, `GetOllamaModels`, `CheckVLLMInstalled`, ... +2 more

**Frameworks:** stdlib-http

### `utils/fileutil/fileutil.go`

**Package:** fileutil

**Functions:** `CheckFileSize`, `SafeReadFile`, `SafeOpenFile`

### `utils/input/handler.go`

**Package:** input

**Types:** `InputType` (type), `ScrapeConfig` (struct), `Input` (struct), `Handler` (struct)

**Functions:** `NewHandler`, `ProcessStdin`, `getMimeType`, `isSourceCode`, `isImageFile`, `ProcessPath`, `containsWildcard`, `processWildcard`, ... +11 more

### `utils/input/validator.go`

**Package:** input

**Types:** `Validator` (struct)

**Functions:** `NewValidator`, `ValidatePath`, `ValidateFileExtension`, `IsImageFile`, `IsDocumentFile`, `IsSourceCodeFile`

### `utils/models/anthropic.go`

**Package:** models

**Types:** `AnthropicProvider` (struct), `anthropicMessage` (struct), `anthropicContent` (struct), `anthropicSource` (struct), `anthropicRequest` (struct), `anthropicResponse` (struct), `AnthropicModel` (struct), `AnthropicModelsResponse` (struct)

**Functions:** `NewAnthropicProvider`, `debugf`, `Name`, `SupportsModel`, `Configure`, `SendPrompt`, `SendPromptWithFile`, `ValidateModel`, ... +5 more

**Frameworks:** stdlib-http

### `utils/models/claudecode.go`

**Package:** models

**Types:** `ClaudeCodeProvider` (struct)

**Functions:** `NewClaudeCodeProvider`, `Name`, `debugf`, `SupportsModel`, `Configure`, `findClaudeBinary`, `SendPrompt`, `SendPromptWithFile`, ... +5 more

### `utils/models/deepseek.go`

**Package:** models

**Types:** `DeepseekProvider` (struct)

**Functions:** `NewDeepseekProvider`, `Name`, `debugf`, `SupportsModel`, `Configure`, `createChatCompletionRequest`, `SendPrompt`, `SendPromptWithFile`, ... +6 more

### `utils/models/geminicli.go`

**Package:** models

**Types:** `GeminiCLIProvider` (struct)

**Functions:** `NewGeminiCLIProvider`, `Name`, `debugf`, `SupportsModel`, `Configure`, `findGeminiBinary`, `SendPrompt`, `SendPromptWithFile`, ... +5 more

### `utils/models/google.go`

**Package:** models

**Types:** `GoogleProvider` (struct)

**Functions:** `NewGoogleProvider`, `Name`, `debugf`, `ValidateModel`, `SupportsModel`, `Configure`, `SendPrompt`, `SendPromptWithFile`, ... +1 more

### `utils/models/moonshot.go`

**Package:** models

**Types:** `MoonshotProvider` (struct)

**Functions:** `NewMoonshotProvider`, `Name`, `debugf`, `SupportsModel`, `Configure`, `createChatCompletionRequest`, `SendPrompt`, `SendPromptWithFile`, ... +9 more

**Frameworks:** stdlib-http

## Operational Notes

**Package:** `go.mod`, `examples/codebase-index/sample-project/go.mod`

**Build:** `Makefile`

## Risk / Caution Areas

**Secrets:**
- `cmd/server.go`
- `cmd/root.go`
- `utils/config/env.go`
- `utils/config/server.go`
- `utils/discovery/discovery.go`
- `utils/input/handler.go`
- `utils/models/anthropic.go`
- `utils/models/claudecode.go`
- `utils/models/deepseek.go`
- `utils/models/geminicli.go`
- `utils/models/google.go`
- `utils/models/moonshot.go`
- `utils/models/ollama.go`
- `utils/models/openai.go`
- `utils/models/openaicodex.go`
- `utils/models/provider.go`
- `utils/codebaseindex/store.go`
- `utils/chunker/chunker.go`
- `utils/codebaseindex/extract.go`
- `utils/processor/memory.go`
- `utils/processor/model_handler.go`
- `utils/processor/responses_handler.go`
- `utils/processor/types.go`
- `utils/server/env_handlers.go`
- `utils/server/generate_handler.go`
- `utils/server/logging.go`
- `utils/server/openai_handlers.go`
- `utils/server/openai_types.go`
- `utils/server/provider_handlers.go`
- `utils/server/server.go`
- `utils/server/types.go`
- `utils/server/utils.go`
- `utils/models/xai.go`
- `utils/server/auth.go`
- `utils/models/vllm.go`

**Crypto:**
- `utils/config/env.go`
- `utils/codebaseindex/store.go`
- `utils/codebaseindex/scan.go`

**Concurrency:**
- `utils/discovery/discovery.go`
- `utils/models/anthropic.go`
- `utils/models/claudecode.go`
- `utils/models/deepseek.go`
- `utils/models/geminicli.go`
- `utils/models/google.go`
- `utils/models/moonshot.go`
- `utils/models/ollama.go`
- `utils/models/openai.go`
- `utils/models/openaicodex.go`
- `utils/codebaseindex/scan.go`
- `utils/codebaseindex/adapter.go`
- `utils/processor/input_handler.go`
- `utils/processor/memory.go`
- `utils/processor/tool_executor.go`
- `utils/processor/spinner.go`
- `utils/processor/agentic_loop.go`
- `utils/server/file_handlers.go`
- `utils/server/handlers.go`
- `utils/server/openai_handlers.go`
- `utils/models/xai.go`
- `utils/models/registry.go`
- `utils/models/vllm.go`

**Database:**
- `utils/processor/database_handler.go`

---

*Index generated at 2026-01-21T03:46:55Z*

*Scan time: 15.93075ms*
