![robot image](../comanda-small.jpg)
# COMandA Examples

This directory contains various examples demonstrating different capabilities of COMandA. The examples are organized into categories based on their primary functionality.

## Categories

### Database Connections (`database-connections/`)
Examples demonstrating database integration:
- `db-example.yaml` - Database read/write operations
- `simpledb.sql` - Sample database schema and data
- Supporting files: `Dockerfile` for test environment

To start up the docker container for testing the database examples, run the following commands (after installing Docker)
```bash
cd examples/database-connections/postgres
docker build -t comanda-postgres .
docker run -d -p 5432:5432 comanda-postgres
```


### Model Examples (`model-examples/`)
Examples demonstrating integration with different AI models:
- `openai-example.yaml` - Basic OpenAI integration example
- `ollama-example.yaml` - Using local Ollama models
- `anthropic-pdf-example.yaml` - Using Anthropic's Claude model with PDF processing
- `google-example.yaml` - Integration with Google's AI models
- `xai-example.yaml` - X.AI model integration example

### File Processing (`file-processing/`)
Examples of file manipulation and processing:
- `analyze-csv.yaml` - Processing CSV data
- `consolidate-example.yaml` - Combining multiple files
- `multi-file-example.yaml` - Working with multiple files
- Supporting files: `harvey1.txt`, `harvey2.txt`, `consolidated.txt`, `test.csv`

### Web Scraping (`web-scraping/`)
Examples of web interaction and scraping:
- `scrape-example.yaml` - Web scraping functionality
- `url-example.yaml` - Working with URLs
- `screenshot-example.yaml` - Taking screenshots of web pages
- `scrape-file-example.yaml` - File-based scraping configuration

### Document Processing (`document-processing/`)
Examples of working with various document formats:
- `xml-example.yaml` - XML file handling
- `google-xml-example.yaml` - Google model XML processing
- `markdown-action-example.yaml` - Markdown file processing
- `test-action.md` - Example markdown action file
- Supporting files: `input.xml`, `output.xml`, `sample.pdf`

### Image Processing (`image-processing/`)
Examples of image-related operations:
- `image-example.yaml` - Basic image processing capabilities
- Supporting files: `image.jpeg`

## Running Examples

You can run any example using:

```bash
comanda process examples/[category]/[example-file].yaml
```

For instance:
```bash
comanda process examples/model-examples/openai-example.yaml
```

Each example includes comments explaining its functionality and any specific requirements (like API keys or local model setup).

## Example Types

1. **Basic Examples**: Start with these to understand core functionality
   - `model-examples/openai-example.yaml`
   - `model-examples/ollama-example.yaml`

2. **Advanced Examples**: Demonstrate more complex features
   - `file-processing/consolidate-example.yaml` (multi-file processing)
   - `web-scraping/screenshot-example.yaml` (browser interaction)
   - `document-processing/markdown-action-example.yaml` (external action files)
   - `database-connections/db-example.yaml` (database operations)

3. **Integration Examples**: Show provider-specific features
   - `model-examples/anthropic-pdf-example.yaml` (PDF processing)
   - `model-examples/google-example.yaml` (Google AI integration)
   - `model-examples/xai-example.yaml` (X.AI integration)

4. **Data Examples**: Demonstrate data processing capabilities
   - 'database-connections/postgres/db-example.yaml' (database operations)

### Test Environment

A Docker environment is provided for testing database operations:

```bash
cd examples/database-connections/postgres
docker build -t comanda-postgres .
docker run -d -p 5432:5432 comanda-postgres
```

This creates a PostgreSQL database with sample customer and order data.

## Contributing

Feel free to add new examples! When contributing:
1. Place your example in the appropriate category directory
2. Include clear comments in your YAML file
3. Update this README with a description of your example
4. Include any necessary supporting files
