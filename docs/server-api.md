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

1. List Providers:
```bash
curl -H "Authorization: Bearer your-token" \
     http://localhost:8080/providers
```

2. Update Provider:
```bash
curl -X PUT \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"name":"openai","apiKey":"your-api-key","models":["gpt-4"]}' \
     http://localhost:8080/providers
```

3. Encrypt Environment:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"password":"your-password"}' \
     http://localhost:8080/env/encrypt
```

4. Create File:
```bash
curl -X POST \
     -H "Authorization: Bearer your-token" \
     -H "Content-Type: application/json" \
     -d '{"path":"example.yaml","content":"your content"}' \
     http://localhost:8080/files
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

// Example usage
async function example() {
  try {
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
