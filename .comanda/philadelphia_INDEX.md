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

- **docs**: documentation
- **tests**: testing
- **utils**: utilities, helpers
- **cmd**: CLI commands, entrypoints

## Entry Points

- `main.go` (package: main)
- `examples/codebase-index/sample-project/cmd/server/main.go` (package: main)

## Key Modules

### cmd

Files:
- `cmd/chart.go`
- `cmd/server.go`
- `cmd/configure.go`
- `cmd/process.go`
- `cmd/root.go`

### examples

Files:
- `examples/codebase-index/sample-project/cmd/server/main.go`
- `examples/codebase-index/sample-project/go.mod`
- `examples/codebase-index/sample-project/internal/config/config.go`
- `examples/codebase-index/sample-project/pkg/handlers/users.go`
- `examples/codebase-index/sample-project/pkg/models/user.go`

### utils

Files:
- `utils/models/xai.go`
- `utils/processor/dsl.go`
- `utils/codebaseindex/adapter.go`
- `utils/codebaseindex/extract.go`
- `utils/codebaseindex/manager.go`
- `utils/codebaseindex/scan.go`
- `utils/codebaseindex/store.go`
- `utils/codebaseindex/synthesize.go`
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
- `utils/models/registry.go`
- `utils/models/vllm.go`
- `utils/processor/action_handler.go`
- `utils/processor/agentic_loop.go`
- `utils/processor/codebase_index_handler.go`
- `utils/processor/database_handler.go`
- `utils/chunker/chunker.go`
- `utils/processor/embedded_guide.go`
- `utils/processor/input_handler.go`
- `utils/processor/memory.go`
- `utils/processor/model_handler.go`
- `utils/processor/output_handler.go`
- `utils/processor/progress.go`
- `utils/processor/responses_handler.go`
- `utils/processor/spinner.go`
- `utils/processor/tool_executor.go`
- `utils/processor/types.go`
- `utils/processor/utils.go`
- `utils/retry/retry.go`
- `utils/scraper/scraper.go`
- `utils/server/auth.go`
- `utils/server/bulk_operations.go`
- `utils/server/env_handlers.go`
- `utils/server/file_backup.go`
- `utils/server/file_handlers.go`
- `utils/server/generate_handler.go`
- `utils/server/handlers.go`
- `utils/server/logging.go`
- `utils/server/openai_handlers.go`
- `utils/server/provider_handlers.go`
- `utils/server/openai_types.go`
- `utils/server/types.go`
- `utils/server/utils.go`
- `utils/server/server.go`

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

### `cmd/configure.go`

**Package:** cmd

**Types:** `OllamaModel` (struct), `VLLMModel` (struct)

**Functions:** `isUnsupportedModel`, `isPrimaryOpenAIModel`, `getOpenAIModelsAndCategorize`, `getOpenAIModels`, `getAnthropicModelsAndCategorize`, `getAnthropicModels`, `getXAIModels`, `getDeepseekModels`, ... +26 more

### `cmd/process.go`

**Package:** cmd

**Functions:** `init`, `parseVarsFlags`

**Frameworks:** cobra

### `cmd/root.go`

**Package:** cmd

**Functions:** `buildGeneratePrompt`, `extractYAMLContent`, `init`, `getVersion`, `Execute`

**Frameworks:** cobra

### `Makefile`

### `go.mod`

### `go.sum`

### `utils/models/xai.go`

**Package:** models

**Types:** `XAIProvider` (struct)

**Functions:** `NewXAIProvider`, `Name`, `debugf`, `SupportsModel`, `Configure`, `estimateTokenCount`, `SendPrompt`, `SendPromptWithFile`, ... +4 more

### `utils/processor/dsl.go`

**Package:** processor

**Types:** `GenerateStepConfig` (struct), `ProcessStepConfig` (struct), `Processor` (struct), `stepResult` (struct)

**Functions:** `UnmarshalYAML`, `isParallelStepGroup`, `parseAgenticLoopBlock`, `isTestMode`, `NewProcessor`, `SetProgressWriter`, `SetLastOutput`, `LastOutput`, ... +18 more

### `utils/codebaseindex/adapter.go`

**Package:** codebaseindex

**Types:** `Adapter` (interface), `Registry` (struct), `GoAdapter` (struct), `PythonAdapter` (struct), `TypeScriptAdapter` (struct), `FlutterAdapter` (struct)

**Functions:** `NewRegistry`, `Register`, `Get`, `All`, `Detect`, `detectAdapter`, `GetByNames`, `CombinedIgnoreDirs`, ... +39 more

### `utils/codebaseindex/extract.go`

**Package:** codebaseindex

**Functions:** `extractSymbols`, `readFilePartial`, `extractGoSymbols`, `buildGoFuncSignature`, `extractGoSymbolsRegex`, `detectGoFrameworks`, `detectGoRiskTags`, `extractPythonSymbols`, ... +10 more

### `utils/codebaseindex/manager.go`

**Package:** codebaseindex

**Types:** `Manager` (struct)

**Functions:** `NewManager`, `Generate`, `detectAdapters`, `GetConfig`, `deriveRepoSlugs`, `getGitRepoName`, `logf`

### `utils/codebaseindex/scan.go`

**Package:** codebaseindex

**Functions:** `scanRepository`, `walkDir`, `processFile`, `selectCandidates`, `scoreFile`, `buildIgnoreDirs`, `buildIgnoreGlobs`, `buildConfigPatterns`, ... +8 more

### `utils/codebaseindex/store.go`

**Package:** codebaseindex

**Functions:** `writeOutput`, `determineOutputPath`, `getConfigStorePath`, `encryptToFile`, `DecryptFromFile`, `Decrypt`, `IsEncrypted`

### `utils/codebaseindex/synthesize.go`

**Package:** codebaseindex

**Types:** `moduleInfo` (struct)

**Functions:** `synthesize`, `writePurpose`, `inferPurpose`, `writeRepoLayout`, `writeTreeNode`, `writeCapabilities`, `inferCapabilities`, `writeEntryPoints`, ... +10 more

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

## Operational Notes

**Build:** `Makefile`

**Package:** `go.mod`, `examples/codebase-index/sample-project/go.mod`

## Risk / Caution Areas

**Crypto:**
- `utils/codebaseindex/scan.go`
- `utils/codebaseindex/store.go`
- `utils/config/env.go`

**Database:**
- `utils/processor/database_handler.go`

**Secrets:**
- `cmd/server.go`
- `cmd/root.go`
- `utils/models/xai.go`
- `utils/codebaseindex/extract.go`
- `utils/codebaseindex/store.go`
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
- `utils/models/vllm.go`
- `utils/chunker/chunker.go`
- `utils/processor/memory.go`
- `utils/processor/model_handler.go`
- `utils/processor/responses_handler.go`
- `utils/processor/types.go`
- `utils/server/auth.go`
- `utils/server/env_handlers.go`
- `utils/server/generate_handler.go`
- `utils/server/logging.go`
- `utils/server/openai_handlers.go`
- `utils/server/provider_handlers.go`
- `utils/server/openai_types.go`
- `utils/server/types.go`
- `utils/server/utils.go`
- `utils/server/server.go`

**Concurrency:**
- `utils/models/xai.go`
- `utils/codebaseindex/adapter.go`
- `utils/codebaseindex/scan.go`
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
- `utils/models/registry.go`
- `utils/models/vllm.go`
- `utils/processor/agentic_loop.go`
- `utils/processor/input_handler.go`
- `utils/processor/memory.go`
- `utils/processor/spinner.go`
- `utils/processor/tool_executor.go`
- `utils/server/file_handlers.go`
- `utils/server/handlers.go`
- `utils/server/openai_handlers.go`

---

*Index generated at 2026-01-21T00:43:25Z*

*Scan time: 7.622958ms*
