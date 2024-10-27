# COMandA (Chain of Models and Actions)

COMandA is a command-line tool that enables the composition of Large Language Model (LLM) operations using a YAML-based Domain Specific Language (DSL). It simplifies the process of creating and managing chains of LLM activities that operate on files and information.

## Features

- 🔗 Chain multiple LLM operations together using simple YAML configuration
- 🤖 Support for multiple LLM providers (OpenAI, Anthropic, Ollama)
- 📄 File-based operations and transformations
- 🖼️ Support for image analysis with vision models (screenshots and common image formats)
- 🌐 Direct URL input support for web content analysis
- 🛠️ Extensible DSL for defining complex workflows
- ⚡ Efficient processing of LLM chains

## Installation

```bash
go install github.com/kris-hansen/comanda@latest
```

Or clone and build from source:

```bash
git clone https://github.com/kris-hansen/comanda.git
cd comanda
go build
```

## Configuration

### Environment File

COMandA uses an environment file to store provider configurations and API keys. By default, it looks for a `.env` file in the current directory. You can specify a custom path using the `COMANDA_ENV` environment variable:

```bash
# Use a specific env file
export COMANDA_ENV=/path/to/your/env/file
comanda process your-dsl-file.yaml

# Or specify it inline
COMANDA_ENV=/path/to/your/env/file comanda process your-dsl-file.yaml
```

### Provider Configuration

Configure your providers and models using the interactive configuration command:

```bash
comanda configure
```

This will prompt you to:
1. Select a provider (OpenAI/Anthropic/Ollama)
2. Enter API key (for OpenAI/Anthropic)
3. Specify model name
4. Select model mode:
   - text: For text-only operations
   - vision: For image analysis capabilities
   - multi: For both text and image operations

You can view your current configuration using:

```bash
comanda configure --list                       
Configuration from .env:

Configured Providers:

ollama:
  - codellama (local)
  - llama3.2 (local)
  - phi3.5 (local)

openai:
  - gpt-4o-mini (external)
    Mode: multi

anthropic:
  - claude-3-opus (external)
  - claude-3-sonnet (external)
  - claude-3-5-haiku (external)
```

To remove a model from the configuration:

```bash
comanda configure --remove <model-name>
```

When configuring a model that already exists, you'll be prompted to update its mode. This allows you to change a model's capabilities without removing and re-adding it.

Example configuration output:
```yaml
providers:
  openai:
    api_key: sk-...
    models:
      - name: gpt-4
        type: external
        mode: text
      - name: gpt-4o
        type: external
        mode: vision
  anthropic:
    api_key: sk-...
    models:
      - name: claude-2
        type: external
        mode: text
  ollama:
    models:
      - name: llama2
        type: local
        mode: text
```

## Usage

### Supported File Types

COMandA supports various file types for input:

- Text files: `.txt`, `.md`, `.yml`, `.yaml`
- Image files: `.png`, `.jpg`, `.jpeg`, `.gif`, `.bmp`
- Web content: Direct URLs to web pages, JSON APIs, or other web resources
- Special inputs: `screenshot` (captures current screen)

When using vision-capable models (like gpt-4o), you can analyze both images and screenshots alongside text content.

Images are automatically optimized for processing:
- Large images are automatically resized to a maximum dimension of 1024px while preserving aspect ratio
- PNG compression is applied to reduce token usage while maintaining quality
- These optimizations help prevent rate limit errors and ensure efficient processing

The screenshot feature allows you to capture the current screen state for analysis. When you specify `screenshot` as the input in your DSL file, COMandA will automatically capture the entire screen and pass it to the specified model for analysis. This is particularly useful for UI analysis, bug reports, or any scenario where you need to analyze the current screen state.

For URL inputs, COMandA automatically:
- Detects and validates URLs in input fields
- Fetches content with appropriate error handling
- Handles different content types (HTML, JSON, plain text)
- Stores content in temporary files with appropriate extensions
- Cleans up temporary files after processing

### Creating DSL Files

Create a YAML file defining your chain of operations:

```yaml
# example.yaml
steps:
  - name: "Summarize Content"
    model: "gpt-4"
    provider: "openai"
    input: 
      file: "input.txt"
    prompt: "Summarize the following content:"
    output:
      file: "summary.txt"

  - name: "Generate Analysis"
    model: "claude-2"
    provider: "anthropic"
    input:
      file: "summary.txt"
    prompt: "Analyze the key points in this summary:"
    output:
      file: "analysis.txt"
```

For image analysis:

```yaml
# image-analysis.yaml
input: "image.png"  # Can be any supported image format
model: "gpt-4o"
action: "Analyze this image and describe what you see in detail."
output: "STDOUT"
```

### Running Commands

Run your DSL file:

```bash
comanda process your-dsl-file.yaml
```

For example:

```bash
./comanda process example-dsl.yaml

Processing DSL file: example-dsl.yaml

Response from gpt-4o-mini:
Based on the company names provided, the following seem more like startups, often characterized by modern, innovative, and tech-oriented names:

1. Quantum Computing Labs
2. Blue Ocean Ventures
3. CloudNine Solutions
[...]

Configuration:
- Model: [gpt-4o-mini]
- Action: [look through these company names and identify which ones seem like startups]
- Output: [STDOUT]
```

### Example YAML Files

The project includes several example YAML files demonstrating different use cases:

#### 1. OpenAI Multi-Step Example (openai-example.yaml)
```yaml
step_one:
  input:
    - examples/example_filename.txt
  model:
    - gpt-4o-mini
  action:
    - look through these company names and identify the top five which seem most likely to be startups
  output:
    - STDOUT

step_two:
  input:
    - STDIN
  model:
    - gpt-4o
  action:
    - for each of these company names provide a snappy tagline that would make them stand out
  output:
    - STDOUT
```
This example shows how to chain multiple steps together, where the output of the first step (STDOUT) becomes the input of the second step (STDIN). To run:
```bash
comanda process examples/openai-example.yaml
```

#### 2. Image Analysis Example (image-example.yaml)
```yaml
step:
  input: examples/image.jpeg
  model: gpt-4o
  action: "Analyze this screenshot and describe what you see in detail."
  output: STDOUT
```
This example demonstrates how to analyze an image file using a vision-capable model. To run:
```bash
comanda process examples/image-example.yaml
```

#### 3. Screenshot Analysis Example (screenshot-example.yaml)
```yaml
step:
  input: screenshot
  model: gpt-4o
  action: "Analyze this screenshot and describe what you see in detail."
  output: STDOUT
```
This example shows how to capture and analyze the current screen state. To run:
```bash
comanda process examples/screenshot-example.yaml
```

#### 4. Local Model Example (ollama-example.yaml)
```yaml
step:
  input: examples/example_filename.txt
  model: llama2
  action: look through these company names and identify the top five which seem most likely in the HVAC business
  output: STDOUT
```
This example demonstrates using a local model through Ollama. Make sure you have Ollama installed and the specified model pulled before running:
```bash
comanda process examples/ollama-example.yaml
```

#### 5. URL Input Example (url-example.yaml)
```yaml
steps:
  analyze_webpage:
    input: https://example.com
    model: gpt-4
    action: Analyze the webpage content and provide a summary
    output: STDOUT

  analyze_api:
    input: https://api.example.com/data.json
    model: gpt-4
    action: Extract key insights from the API response
    output: analysis.txt
```
This example shows how to analyze web content directly from URLs. The processor automatically handles different content types and stores them appropriately. To run:
```bash
comanda process examples/url-example.yaml
```

## Project Structure

```
comanda/
├── cmd/                    # Command line interface
├── utils/
│   ├── config/            # Configuration handling
│   ├── input/             # Input validation and processing
│   ├── models/            # LLM provider implementations
│   └── processor/         # DSL processing logic
├── go.mod
├── go.sum
└── main.go
```

## Contributing

Contributions are welcome! Here's how you can help:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please ensure your PR:

- Includes tests for new functionality
- Updates documentation as needed
- Follows the existing code style
- Includes a clear description of the changes

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- OpenAI and Anthropic for their LLM APIs
- The Ollama project for local LLM support
- The Go community for excellent libraries and tools
