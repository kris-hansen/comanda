![robot image](comanda-small.jpg)
# COMandA (Chain of Models and Actions)

COMandA is a command-line tool that enables the composition of Large Language Model (LLM) operations using a YAML-based Domain Specific Language (DSL). It simplifies the process of creating and managing agentic workflows composed of downloads, files, text, images, documents, multiple providers and multiple models.

Create YAML 'recipes' and use `comanda process` to execute the recipe file.

COMandA allows you to use the best provider and model for each step and compose information pipelines that combine the stregths of different LLMs. It supports multiple LLM providers (OpenAI, Anthropic, Google, X.AI, Ollama) and provides extensible DSL capabilities for defining complex information workflows.

## Features

- 🔗 Chain multiple LLM operations together using simple YAML configuration
- 🤖 Support for multiple LLM providers (OpenAI, Anthropic, Google, X.AI, Ollama)
- 📄 File-based operations and transformations
- 🖼️ Support for image analysis with vision models (screenshots and common image formats)
- 🌐 Direct URL input support for web content analysis
- 🕷️ Advanced web scraping capabilities with configurable options
- 🛠️ Extensible DSL for defining complex workflows
- ⚡ Efficient processing of LLM chains
- 🔒 HTTP server mode with bearer token authentication
- 🔐 Secure configuration encryption for protecting API keys and secrets
- 📁 Multi-file input support with content consolidation
- 📝 Markdown file support for reusable actions (prompts)

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

### Configuration Encryption

COMandA supports encrypting your configuration file to protect sensitive information like API keys. The encryption uses AES-256-GCM with password-derived keys, providing strong security against unauthorized access.

To encrypt your configuration:
```bash
comanda configure --encrypt
```

You'll be prompted to enter and confirm an encryption password. Once encrypted, all commands that need to access the configuration (process, serve, configure) will prompt for the password.

Example workflow:
```bash
# First, configure your providers and API keys
comanda configure

# Then encrypt the configuration
comanda configure --encrypt
Enter encryption password: ********
Confirm encryption password: ********
Configuration encrypted successfully!

# When running commands, you'll be prompted for the password
comanda process your-dsl-file.yaml
Enter decryption password: ********
```

The encryption system provides:
- AES-256-GCM encryption (industry standard)
- Password-based key derivation
- Protection against tampering
- Brute-force resistance

You can still view your configuration using:
```bash
comanda configure --list
```
This will prompt for the password if the configuration is encrypted.

### Provider Configuration

Configure your providers and models using the interactive configuration command:

```bash
comanda configure
```

This will prompt you to:

1. Select a provider (OpenAI/Anthropic/Google/X.AI/Ollama)
2. Enter API key (for OpenAI/Anthropic/Google/X.AI)
3. Specify model name
4. Select model mode:
   - text: For text-only operations
   - vision: For image analysis capabilities
   - multi: For both text and image operations

You can view your current configuration using:

```bash
comanda configure --list                       
Server Configuration:
Port: 8080
Data Directory: data
Authentication Enabled: true

Configured Providers:

anthropic:
  - claude-3-5-latest (external)

google:
  - gemini-pro (external)

ollama:
  - llama3.2 (local)

openai:
  - gpt-4o-mini (external)
  - gpt-4o (external)

xai:
  - grok-beta (external)
```

To remove a model from the configuration:

```bash
comanda configure --remove <model-name>
```

To update an API key for a provider (e.g., after key rotation):

```bash
comanda configure --update-key=<provider-name>
```

This will prompt you for the new API key and update it in the configuration. For example:

```bash
comanda configure --update-key=openai
Enter new API key: sk-...
Successfully updated API key for provider 'openai'
```

When configuring a model that already exists, you'll be prompted to update its mode. This allows you to change a model's capabilities without removing and re-adding it.

Example configuration output:

``` yaml
ollama:
  - llama3.2 (local)
    Modes: text

openai:
  - gpt-4o-mini (external)
    Modes: text
  - gpt-4o (external)
    Modes: text, vision

xai:
  - grok-beta (external)
    Modes: text

anthropic:
  - claude-3-5-sonnet-latest (external)
    Modes: text, file
  - claude-3-5-haiku-latest (external)
    Modes: text, file
  - claude-3-5-sonnet-20241022 (external)
    Modes: text, file

google:
  - gemini-pro (external)
    Modes: text
  - gemini-1.5-pro (external)
    Modes: text, file, vision
```

### Server Configuration

COMandA can run as an HTTP server, allowing you to process chains of models and actions defined in YAML files via HTTP requests. To configure the server:

```bash
comanda configure --server
```

This will prompt you to:
1. Set the server port (default: 8080)
2. Set the data directory path (default: data)
3. Generate a bearer token for authentication
4. Enable/disable authentication

The server configuration is stored in your `.env` file alongside provider and model settings:

```yaml
server:
  port: 8080
  data_dir: "examples"  # Directory containing YAML files to process
  bearer_token: "your-generated-token"
  enabled: true  # Whether authentication is required
```

To start the server:

```bash
comanda serve
```

The server provides the following endpoints:

### 1. Process Endpoint

`GET /process` processes a YAML file from the configured data directory:

```bash
# Without authentication
curl "http://localhost:8080/process?filename=openai-example.yaml"

# With authentication (when enabled)
curl -H "Authorization: Bearer your-token" "http://localhost:8080/process?filename=openai-example.yaml"
```

Response format:
```json
{
  "success": true,
  "message": "Successfully processed openai-example.yaml",
  "output": "Response from gpt-4o-mini:\n..."
}
```

Error response:
```json
{
  "success": false,
  "error": "Error message here",
  "output": "Any output generated before the error"
}
```

### 2. List Endpoint

`GET /list` returns a list of YAML files in the configured data directory:

```bash
curl -H "Authorization: Bearer your-token" "http://localhost:8080/list"
```

Response format:
```json
{
  "success": true,
  "files": [
    "openai-example.yaml",
    "image-example.yaml",
    "screenshot-example.yaml"
  ]
}
```

### 3. Health Check Endpoint

`GET /health` returns the server's current status:

```bash
curl "http://localhost:8080/health"
```

Response format:
```json
{
  "status": "ok",
  "timestamp": "2024-11-02T20:39:13Z"
}
```

The server logs all requests to the console, including:
- Timestamp
- Request method and path
- Query parameters
- Authorization header (token masked)
- Response status code
- Request duration

Example server log:
```
2024/11/02 21:06:33 Request: method=GET path=/health query= auth=Bearer ******** status=200 duration=875µs
2024/11/02 21:06:37 Request: method=GET path=/list query= auth=Bearer ******** status=200 duration=812.208µs
2024/11/02 21:06:45 Request: method=GET path=/process query=filename=examples/openai-example.yaml auth=Bearer ******** status=200 duration=3.360269792s
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
summarize:
    model: "gpt-4"
    provider: "openai"
    input: 
      file: "input.txt"
    prompt: "Summarize the following content:"
    output:
      file: "summary.txt"

analyze:
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
analyze:
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
Processing DSL file: examples/openai-example.yaml

Configuration:

Step: step_one
- Input: [examples/example_filename.txt]
- Model: [gpt-4o-mini]
- Action: [look through these company names and identify the top five which seem most likely in the HVAC business]
- Output: [STDOUT]

Step: step_two
- Input: [STDIN]
- Model: [gpt-4o]
- Action: [for each of these company names provide a snappy tagline that would make them stand out]
- Output: [STDOUT]


Response from gpt-4o-mini:
Based on the company names provided, the following five seem most likely to be in the HVAC (Heating, Ventilation, and Air Conditioning) business:

1. **Evergreen Industries** - The name suggests a focus on sustainability, which is often associated with HVAC systems that promote energy efficiency.

2. **Mountain Peak Investments** - While not directly indicative of HVAC, the name suggests a focus on construction or infrastructure, which often involves HVAC installations.

3. **Cascade Technologies** - The term "cascade" could relate to water systems or cooling technologies, which are relevant in HVAC.

4. **Summit Technologies** - Similar to Mountain Peak, "Summit" may imply involvement in high-quality or advanced systems, possibly including HVAC solutions.

5. **Zenith Industries** - The term "zenith" suggests reaching the highest point, which can be associated with premium or top-tier HVAC products or services.

These names suggest a connection to industries related to heating, cooling, or building systems, which are integral to HVAC.

Response from gpt-4o:
Certainly! Here are some snappy taglines for each of the company names that could help them stand out in the HVAC industry:

1. **Evergreen Industries**: "Sustainability in Every Breath."

2. **Mountain Peak Investments**: "Building Comfort from the Ground Up."

3. **Cascade Technologies**: "Cooling Solutions That Flow."

4. **Summit Technologies**: "Reaching New Heights in HVAC Innovation."

5. **Zenith Industries**: "At the Pinnacle of Climate Control Excellence."
```

### Example YAML Files

Currently the key tags in the YAML files are `stepname` (can be anything), `input`, `model`, `action`, `output` - CoMandA will parse and process based on these tags.

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
scrape_webpage:
    input: https://example.com
    model: gpt-4o-mini
    action: Analyze the scraped content and provide insights
    output: STDOUT
    scrape_config:
        allowed_domains:
            - example.com
        headers:
            Accept: text/html,application/xhtml+xml
            Accept-Language: en-US,en;q=0.9
        extract:
            - title     # Extract page title
            - text      # Extract text content from paragraphs
            - links     # Extract URLs from anchor tags
```

This example shows how to analyze web content with advanced scraping capabilities. The `scrape_config` tag allows you to configure:
- Domain restrictions with `allowed_domains`
- Custom HTTP headers
- Specific elements to extract (title, text content, links)

To run:

```bash
comanda process examples/url-example.yaml
```

#### 6. X.AI Example (xai-example.yaml)

```yaml
step_one:
  input:
    - examples/example_filename.txt
  model:
    - grok-beta
  action:
    - analyze these company names and identify which ones have the strongest brand potential
  output:
    - STDOUT

step_two:
  input:
    - STDIN
  model:
    - grok-beta
  action:
    - for each of these companies, suggest a modern social media marketing strategy
  output:
    - STDOUT
```

This example demonstrates using X.AI's grok-beta model. Make sure you have configured your X.AI API key before running:

```bash
comanda process examples/xai-example.yaml
```

#### 7. Multi-File Consolidation Example (consolidate-example.yaml)

```yaml
step_one:
  input: NA
  model: gpt-4o-mini
  action: "write a first paragraph about a snail named Harvey"
  output: examples/harvey1.txt

step_two:
  input: NA
  model: gpt-4o-mini
  action: "write a second paragraph about a snail named Harvey"
  output: examples/harvey2.txt

step_three:
  input: "filenames: examples/harvey1.txt,examples/harvey2.txt"
  model: gpt-4o-mini
  action: "Read both files and combine their contents into a single consolidated story"
  output: examples/consolidated.txt
```

This example demonstrates the multi-file input feature, where multiple files can be processed together. The special `filenames:` prefix in the input field allows you to specify a comma-separated list of files to be processed as a single input. To run:

```bash
comanda process examples/consolidate-example.yaml
```

#### 8. Markdown Action Example (markdown-action-example.yaml)

The action stage of a step can be a markdown file. This allows you to store complex prompts or actions in separate files for better organization and reuse.

```yaml
step1:
  input: examples/test.csv
  model: gpt-4
  action: examples/test-action.md
  output: STDOUT
```

This example demonstrates using a markdown file as an action. Instead of specifying the action directly in the YAML file, you can reference a markdown file that contains the action text. This is particularly useful for:
- Reusing common actions across multiple steps or files
- Storing complex prompts in separate files for better organization
- Version controlling your prompts alongside your code
- Making actions more maintainable and easier to edit

The contents of test-action.md:
```markdown
Analyze this input and provide a detailed summary of its key points and main themes.
```

To run:
```bash
comanda process examples/markdown-action-example.yaml
```

## Project Structure

```bash
comanda/
├── cmd/                    # Command line interface
├── utils/
│   ├── config/            # Configuration handling
│   ├── input/             # Input validation and processing
│   ├── models/            # LLM provider implementations
│   ├── scraper/           # Web scraping functionality
│   └── processor/         # DSL processing logic
├── go.mod
├── go.sum
└── main.go
```

## Roadmap

The following features are being considered:

- More providers:
  - Huggingface inference API?
  - Image generation providers?
  - others?
- URL output support, post this data to URL
  - Need to add credential support
  - Need to solve for local secrets encryption
- Branching and basic if/or logic
- Routing logic i.e., use this model if the output is x and that model if y

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

## Citation

If you use COMandA in your research or academic work, please cite it as follows:

### BibTeX
```bibtex
@software{comanda2024,
  author       = {Hansen, Kris},
  title        = {COMandA: Chain of Models and Actions},
  year         = {2024},
  publisher    = {GitHub},
  url          = {https://github.com/kris-hansen/comanda},
  description  = {A command-line tool for composing Large Language Model operations using a YAML-based DSL}
}
```

## Acknowledgments

- OpenAI and Anthropic for their LLM APIs
- The Ollama project for local LLM support
- The Go community for excellent libraries and tools
- The Colly framework for web scraping capabilities
