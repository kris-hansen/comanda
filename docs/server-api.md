# Comanda Server API Documentation

## Overview

The Comanda server provides a RESTful API interface for managing providers, environment configuration, and file operations. The goal is to have CLI and server functionality at parity so that you can build your own UI for managing Comanda agentic workflows.

## Authentication

When authentication is enabled, all endpoints require a Bearer token in the Authorization header:
```http
Authorization: Bearer your-token
```

## API Endpoints

### Provider Management

The provider management API allows you to configure and manage different AI model providers (OpenAI, Anthropic, Google, etc.).

#### List Providers
```http
GET /providers
Authorization: Bearer <token>
```

Lists all configured providers and their available models.

Response:
```json
{
  "success": true,
  "providers": [
    {
      "name": "openai",
      "models": ["gpt-4", "gpt-3.5-turbo"],
      "enabled": true
    },
    {
      "name": "anthropic",
      "models": ["claude-2", "claude-instant"],
      "enabled": true
    }
  ]
}
```

#### Update Provider
```http
PUT /providers
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "openai",
  "apiKey": "your-api-key",
  "models": ["gpt-4", "gpt-3.5-turbo"],
  "enabled": true
}
```

Updates or adds a provider configuration. If the provider doesn't exist, it will be created.

Response:
```json
{
  "success": true,
  "message": "Provider openai updated successfully"
}
```

#### Delete Provider
```http
DELETE /providers/{provider_name}
Authorization: Bearer <token>
```

Removes a provider configuration.

Response:
```json
{
  "success": true,
  "message": "Provider openai removed successfully"
}
```

### Environment Security

The environment security API provides endpoints for encrypting and decrypting the environment configuration file.

#### Encrypt Environment
```http
POST /env/encrypt
Authorization: Bearer <token>
Content-Type: application/json

{
  "password": "your-password"
}
```

Encrypts the environment file with the provided password. The original file will be replaced with an encrypted version.

Response:
```json
{
  "success": true,
  "message": "Environment file encrypted successfully"
}
```

#### Decrypt Environment
```http
POST /env/decrypt
Authorization: Bearer <token>
Content-Type: application/json

{
  "password": "your-password"
}
```

Decrypts the environment file using the provided password. The encrypted file will be replaced with the decrypted version.

Response:
```json
{
  "success": true,
  "message": "Environment file decrypted successfully"
}
```

### File Operations

The file operations API provides endpoints for managing files with enhanced metadata.

#### List Files
```http
GET /list
Authorization: Bearer <token>
```

Returns a list of files with detailed metadata including creation date, modification date, and supported methods.

Response:
```json
{
  "success": true,
  "files": [
    {
      "name": "example.yaml",
      "path": "example.yaml",
      "size": 1234,
      "isDir": false,
      "createdAt": "2024-03-21T10:00:00Z",
      "modifiedAt": "2024-03-21T10:00:00Z",
      "methods": "GET"
    }
  ]
}
```

The `methods` field indicates whether a YAML file accepts input:
- `GET`: File can be processed without input
- `POST`: File requires input for processing

#### Create File
```http
POST /files
Authorization: Bearer <token>
Content-Type: application/json

{
  "path": "example.yaml",
  "content": "your file content"
}
```

Creates a new file with the specified content. The path must be relative to the data directory.

Response:
```json
{
  "success": true,
  "message": "File created successfully",
  "file": {
    "name": "example.yaml",
    "path": "example.yaml",
    "size": 1234,
    "isDir": false,
    "createdAt": "2024-03-21T10:00:00Z",
    "modifiedAt": "2024-03-21T10:00:00Z"
  }
}
```

#### Update File
```http
PUT /files?path=example.yaml
Authorization: Bearer <token>
Content-Type: application/json

{
  "content": "updated content"
}
```

Updates an existing file with new content.

Response:
```json
{
  "success": true,
  "message": "File updated successfully",
  "file": {
    "name": "example.yaml",
    "path": "example.yaml",
    "size": 1234,
    "isDir": false,
    "createdAt": "2024-03-21T10:00:00Z",
    "modifiedAt": "2024-03-21T10:00:00Z"
  }
}
```

#### Delete File
```http
DELETE /files?path=example.yaml
Authorization: Bearer <token>
```

Deletes the specified file.

Response:
```json
{
  "success": true,
  "message": "File deleted successfully"
}
```

#### Upload File
```http
POST /files/upload?runtimeDir=uploads
Authorization: Bearer <token>
Content-Type: multipart/form-data

Form fields:
- file: (binary file data)
- path: "path/to/file.ext"
```

Uploads a file using multipart/form-data format. The file will be saved at the specified path. The optional `runtimeDir` query parameter specifies a subdirectory within the data directory for organizing uploads.

Response:
```json
{
  "success": true,
  "message": "File uploaded successfully"
}
```

#### Get File Content
```http
GET /files/content?path=example.txt
Authorization: Bearer <token>
Accept: text/plain
```

Retrieves the content of a file as plain text.

Response:
```text
File content as plain text
```

#### Download File
```http
GET /files/download?path=example.pdf
Authorization: Bearer <token>
Accept: application/octet-stream
```

Downloads a file in binary format. The response will be the raw file content with appropriate content type.

Response: Binary file content

### Health Check

#### Get Server Health
```http
GET /health
Authorization: Bearer <token>
```

Returns the current health status of the server.

Response:
```json
{
  "success": true,
  "message": "Server is healthy",
  "statusCode": 200,
  "response": "OK"
}
```

### YAML Operations

#### Upload YAML
```http
POST /yaml/upload?runtimeDir=myproject
Authorization: Bearer <token>
Content-Type: application/json

{
  "content": "your yaml content here"
}
```

Uploads a YAML file for processing. The optional `runtimeDir` query parameter specifies a subdirectory within the data directory for organizing YAML scripts.

Response:
```json
{
  "success": true,
  "message": "YAML file uploaded successfully"
}
```

#### Process YAML
```http
POST /yaml/process?runtimeDir=myproject
Authorization: Bearer <token>
Content-Type: application/json

# Regular processing (JSON response)
{
  "content": "your yaml content here",
  "streaming": false
}

# Streaming processing (Server-Sent Events)
{
  "content": "your yaml content here",
  "streaming": true
}
```

For streaming requests, also include:
```http
Accept: text/event-stream
```

Regular processing response:
```json
{
  "success": true,
  "yaml": "processed yaml content"
}
```

Streaming response (Server-Sent Events):
```
data: Processing step 1...

data: Model response: ...

data: Processing step 2...

data: Processing complete
```

### Generate Endpoint

The generate endpoint allows you to generate Comanda workflow YAML files using an LLM based on natural language prompts.

#### Generate Workflow
```http
POST /generate
Authorization: Bearer <token>
Content-Type: application/json

{
  "prompt": "Create a workflow to summarize a file and save it",
  "model": "gpt-4o-mini"  # Optional, uses default_generation_model if not specified
}
```

Generates a new Comanda workflow YAML file based on the provided prompt.

Request parameters:
- `prompt` (required): Natural language description of the workflow you want to create
- `model` (optional): Specific model to use for generation. If not provided, uses the `default_generation_model` from configuration

Response:
```json
{
  "success": true,
  "yaml": "step_one:\n  model: gpt-4o-mini\n  input: FILE\n  ...",
  "model": "gpt-4o-mini"
}
```

Error response:
```json
{
  "success": false,
  "error": "No model specified and no default_generation_model configured"
}
```

### Process Endpoint

The process endpoint handles YAML file processing via POST requests only, supporting both regular and streaming responses.

#### Process File (POST)
```http
POST /process?filename=example.yaml&runtimeDir=myproject
Authorization: Bearer <token>
Content-Type: application/json

# Process a YAML file with input
POST /process?filename=example.yaml&runtimeDir=myproject
Authorization: Bearer <token>
Content-Type: application/json

{
  "input": "your input here",
  "streaming": false  # Set to true for Server-Sent Events streaming
}

# For streaming responses, include:
Accept: text/event-stream

# Response formats:

# Regular JSON response (streaming: false):
{
  "success": true,
  "message": "Successfully processed example.yaml",
  "output": "Response from gpt-4o-mini:\n..."
}

# Server-Sent Events response (streaming: true):
data: Processing step 1...

data: Model response: ...

data: Processing step 2...

data: Processing complete

# Error response (method not allowed):
{
  "success": false,
  "error": "YAML processing is only available via POST requests. Please use POST with your YAML content."
}
```

Note: All YAML processing must be done via POST requests. The endpoint no longer supports GET requests for processing.

## Security Features

### Authentication
- Bearer token authentication when enabled
- Token must be provided in the Authorization header
- All endpoints check authentication if enabled

### File Security
- Path traversal prevention
- Files are restricted to the data directory
- Proper permission checks on file operations

### Environment Security
- Password-based encryption using AES-256-GCM
- Secure key derivation using SHA-256
- Base64 encoding for encrypted data

## Error Handling

All endpoints return appropriate HTTP status codes and JSON responses with error messages:

```json
{
  "success": false,
  "error": "Detailed error message"
}
```

Common status codes:
- 200: Success
- 400: Bad Request (invalid input)
- 401: Unauthorized (missing or invalid token)
- 403: Forbidden (path traversal attempt)
- 404: Not Found (file or resource not found)
- 409: Conflict (file already exists)
- 500: Internal Server Error

## Example Usage

### Using curl

1. Process YAML with streaming:
```bash
# Process YAML content with streaming
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -H "Accept: text/event-stream" \
     -d '{"content":"your yaml content", "streaming": true}' \
     http://localhost:8080/yaml/process

# Process file with streaming
curl -H "Authorization: Bearer your-token" \
     -H "Accept: text/event-stream" \
     "http://localhost:8080/process?filename=example.yaml&streaming=true"

# Process file with input and streaming
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -H "Accept: text/event-stream" \
     -d '{"input":"your input here", "streaming": true}' \
     "http://localhost:8080/process?filename=example.yaml"
```

2. List Providers:
```bash
curl -H "Authorization: Bearer your-token" \
     http://localhost:8080/providers
```

3. Update Provider:
```bash
curl -X PUT \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"name":"openai","apiKey":"your-api-key","models":["gpt-4"]}' \
     http://localhost:8080/providers
```

4. Encrypt Environment:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"password":"your-password"}' \
     http://localhost:8080/env/encrypt
```

5. Create File:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"path":"example.yaml","content":"your content"}' \
     http://localhost:8080/files
```

6. Upload File:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -F "file=@/path/to/local/file.txt" \
     -F "path=destination/file.txt" \
     http://localhost:8080/files/upload
```

7. Get File Content:
```bash
curl -H "Authorization: Bearer your-token" \
     -H "Accept: text/plain" \
     http://localhost:8080/files/content?path=example.txt
```

8. Download File:
```bash
curl -H "Authorization: Bearer your-token" \
     -H "Accept: application/octet-stream" \
     http://localhost:8080/files/download?path=example.pdf \
     --output downloaded_file.pdf
```

9. Generate Workflow:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"prompt":"Create a workflow to read a CSV file and summarize its contents"}' \
     http://localhost:8080/generate

# With specific model
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"prompt":"Create a workflow to analyze sentiment from user input","model":"gpt-4o"}' \
     http://localhost:8080/generate
```

### Using JavaScript

```javascript
// Base configuration
const API_URL = 'http://localhost:8080';
const TOKEN = 'your-token';

const headers = {
  'Authorization': `Bearer ${TOKEN}`,
  'Content-Type': 'application/json'
};

// Process YAML with streaming
async function processYAMLStreaming(content) {
  const response = await fetch(`${API_URL}/yaml/process`, {
    method: 'POST',
    headers: {
      ...headers,
      'Accept': 'text/event-stream'
    },
    body: JSON.stringify({ content, streaming: true })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    
    const text = decoder.decode(value);
    // Handle each SSE message
    console.log(text);
  }
}

// Process file with streaming
async function processFileStreaming(filename, input = null) {
  const url = `${API_URL}/process?filename=${encodeURIComponent(filename)}${input ? '' : '&streaming=true'}`;
  const options = {
    method: input ? 'POST' : 'GET',
    headers: {
      ...headers,
      'Accept': 'text/event-stream'
    }
  };
  
  if (input) {
    options.body = JSON.stringify({ input, streaming: true });
  }
  
  const response = await fetch(url, options);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    
    const text = decoder.decode(value);
    // Handle each SSE message
    console.log(text);
  }
}

// List providers
async function listProviders() {
  const response = await fetch(`${API_URL}/providers`, { headers });
  return await response.json();
}

// Update provider
async function updateProvider(provider) {
  const response = await fetch(`${API_URL}/providers`, {
    method: 'PUT',
    headers,
    body: JSON.stringify(provider)
  });
  return await response.json();
}

// Encrypt environment
async function encryptEnvironment(password) {
  const response = await fetch(`${API_URL}/env/encrypt`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ password })
  });
  return await response.json();
}

// Create file
async function createFile(path, content) {
  const response = await fetch(`${API_URL}/files`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ path, content })
  });
  return await response.json();
}

// Update file
async function updateFile(path, content) {
  const response = await fetch(`${API_URL}/files?path=${encodeURIComponent(path)}`, {
    method: 'PUT',
    headers,
    body: JSON.stringify({ content })
  });
  return await response.json();
}

// Delete file
async function deleteFile(path) {
  const response = await fetch(`${API_URL}/files?path=${encodeURIComponent(path)}`, {
    method: 'DELETE',
    headers
  });
  return await response.json();
}

// Upload file
async function uploadFile(file, path) {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('path', path);

  const response = await fetch(`${API_URL}/files/upload`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${TOKEN}`
    },
    body: formData
  });
  return await response.json();
}

// Get file content
async function getFileContent(path) {
  const response = await fetch(`${API_URL}/files/content?path=${encodeURIComponent(path)}`, {
    headers: {
      'Authorization': `Bearer ${TOKEN}`,
      'Accept': 'text/plain'
    }
  });
  return await response.text();
}

// Download file
async function downloadFile(path) {
  const response = await fetch(`${API_URL}/files/download?path=${encodeURIComponent(path)}`, {
    headers: {
      'Authorization': `Bearer ${TOKEN}`,
      'Accept': 'application/octet-stream'
    }
  });
  return await response.blob();
}

// Generate workflow
async function generateWorkflow(prompt, model = null) {
  const body = { prompt };
  if (model) {
    body.model = model;
  }
  
  const response = await fetch(`${API_URL}/generate`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body)
  });
  return await response.json();
}

// Example usage
async function example() {
  try {
    // Process YAML with streaming
    await processYAMLStreaming(`
      step_one:
        model: gpt-4o
        input: "Hello"
        output: STDOUT
    `);

    // Process file with streaming
    await processFileStreaming('example.yaml', 'optional input here');

    // List providers
    const providers = await listProviders();
    console.log('Providers:', providers);

    // Update OpenAI provider
    const updateResult = await updateProvider({
      name: 'openai',
      apiKey: 'your-api-key',
      models: ['gpt-4', 'gpt-3.5-turbo'],
      enabled: true
    });
    console.log('Update result:', updateResult);

    // Create a file
    const createResult = await createFile('example.yaml', 'file content');
    console.log('Create result:', createResult);

    // Generate a workflow
    const generateResult = await generateWorkflow(
      'Create a workflow to read a CSV file and summarize its contents'
    );
    console.log('Generated YAML:', generateResult.yaml);

    // Generate with specific model
    const generateWithModel = await generateWorkflow(
      'Create a workflow to analyze sentiment from user input',
      'gpt-4o'
    );
    console.log('Generated with model:', generateWithModel.model);
  } catch (error) {
    console.error('Error:', error);
  }
}
```

## Best Practices

1. Always handle errors appropriately:
   - Check response status codes
   - Parse error messages from responses
   - Implement proper error handling in your code

2. Secure your bearer token:
   - Never expose it in client-side code
   - Use environment variables or secure configuration
   - Rotate tokens periodically

3. File operations:
   - Always use relative paths
   - Validate file content before sending
   - Handle large files appropriately

4. Environment security:
   - Use strong passwords for encryption
   - Store passwords securely
   - Keep backup of environment file before encryption

5. Provider management:
   - Validate API keys before saving
   - Keep track of enabled/disabled providers
   - Monitor model availability

6. Streaming:
   - Use streaming for long-running operations
   - Handle SSE events appropriately
   - Implement proper error handling for stream disconnections
   - Consider fallback to non-streaming for older browsers
