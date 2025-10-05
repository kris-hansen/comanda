# Integration Tests

This directory contains integration tests for the comanda project. These tests verify that the compiled binary works correctly with real example workflows.

## Prerequisites

To run integration tests, you need:

1. **Go 1.23 or higher** installed
2. **API Keys** configured in `.env` file at the project root:
   - `OPENAI_API_KEY` for OpenAI tests
   - `ANTHROPIC_API_KEY` for Anthropic tests (optional)
   - Database credentials if testing database features
3. **PostgreSQL** (optional, for database tests)

## Running Tests

### Using Make

The recommended way to run integration tests:

```bash
# Run all integration tests
make integration
```

### Using Go directly

You can also run tests directly with Go:

```bash
# Run from project root
cd tests/integration
go test -v -tags=integration ./...
```

## Test Coverage

The integration tests cover:

1. **Binary Build** (`TestBinaryBuild`)
   - Verifies the binary compiles successfully
   - Ensures the executable is created

2. **Example Workflows**
   - `TestOpenAIExample` - Tests basic OpenAI workflow
   - `TestFileConsolidation` - Tests file processing and consolidation
   - `TestParallelProcessing` - Tests parallel execution

3. **Database Operations** (`TestDatabaseExample`)
   - Tests database read/write operations
   - Requires PostgreSQL to be running
   - Automatically skipped if database is not configured

4. **CLI Commands**
   - `TestConfigureCommand` - Tests configuration listing
   - `TestVersionCommand` - Tests version display

## Skipping Tests

Tests are automatically skipped when:

- Required API keys are not present in `.env`
- Database is not configured (for database tests)
- Example files are not found

You'll see messages like:
```
--- SKIP: TestOpenAIExample (0.00s)
    examples_test.go:XX: Skipping OpenAI example test: OPENAI_API_KEY not set
```

## Database Test Setup

To run database tests, you need a PostgreSQL instance:

```bash
# Using Docker (recommended)
cd examples/database-connections/postgres
docker build -t comanda-postgres .
docker run -d -p 5432:5432 comanda-postgres
```

Then configure the database in comanda:

```bash
comanda configure --database
```

## CI/CD Integration

Integration tests are designed to be run locally but can be integrated into CI/CD pipelines by:

1. Setting required environment variables as secrets
2. Using the `make integration` command
3. Ensuring the `.env` file is properly configured

Example for GitHub Actions:

```yaml
- name: Run Integration Tests
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: |
    echo "OPENAI_API_KEY=${OPENAI_API_KEY}" > .env
    make integration
```

## Adding New Tests

When adding new integration tests:

1. Use the `//go:build integration` tag at the top of the file
2. Check for required environment variables using `hasRequiredEnvVars()`
3. Skip tests gracefully when prerequisites are not met
4. Clean up any created files/resources after tests
5. Use the `buildTestBinary()` helper to build the binary for testing

Example:

```go
//go:build integration
// +build integration

func TestMyNewFeature(t *testing.T) {
    if !hasRequiredEnvVars(t, "MY_API_KEY") {
        t.Skip("Skipping test: MY_API_KEY not set")
    }

    binaryPath := buildTestBinary(t)
    defer os.Remove(binaryPath)

    // Your test code here
}
```

## Troubleshooting

### Tests are being skipped

Check that:
- `.env` file exists in project root
- Required API keys are present in `.env`
- You're running tests with the `integration` build tag

### Database tests fail

Ensure:
- PostgreSQL is running and accessible
- Database is configured with `comanda configure --database`
- Connection details in `.env` are correct

### Binary build fails

Check:
- Go version is 1.23 or higher
- All dependencies are installed (`make deps`)
- No syntax errors in the code (`make lint`)
