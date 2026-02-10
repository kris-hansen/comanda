# Contributing to COMandA

First off, thank you for considering contributing to COMandA! It's people like you that make COMandA such a great tool.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Code Quality Standards](#code-quality-standards)
- [Testing Guidelines](#testing-guidelines)
- [Submitting Changes](#submitting-changes)
- [Reporting Bugs](#reporting-bugs)
- [Suggesting Enhancements](#suggesting-enhancements)

## Code of Conduct

This project and everyone participating in it is governed by a simple principle: be respectful and constructive. By participating, you are expected to uphold this standard.

## Getting Started

### Prerequisites

Before you begin, ensure you have:

- Go 1.23 or higher installed
- Git installed and configured
- (Optional but recommended) Make installed
- A GitHub account

### Development Setup

1. **Fork the repository** on GitHub

2. **Clone your fork locally**
   ```bash
   git clone https://github.com/YOUR-USERNAME/comanda.git
   cd comanda
   ```

3. **Add the upstream repository**
   ```bash
   git remote add upstream https://github.com/kris-hansen/comanda.git
   ```

4. **Install dependencies**
   ```bash
   make deps
   ```

5. **Build the project**
   ```bash
   make build
   ```

6. **Run tests to verify setup**
   ```bash
   make test
   ```

## Making Changes

### Creating a Branch

Always create a new branch for your changes:

```bash
# Update your local main branch
git checkout main
git pull upstream main

# Create a new feature branch
git checkout -b feature/your-feature-name

# Or for bug fixes
git checkout -b fix/bug-description
```

### Branch Naming Convention

- `feature/` - for new features
- `fix/` - for bug fixes
- `docs/` - for documentation changes
- `refactor/` - for code refactoring
- `test/` - for adding or updating tests

## Code Quality Standards

We maintain high code quality standards using automated tools and manual reviews.

### Linting

All code must pass linting checks:

```bash
# Run linter with auto-fix
make lint

# Check without fixing (what CI will run)
make lint-check
```

Our linting configuration (`.golangci.yml`) enforces:

- **Error handling**: All errors must be checked
- **Code simplification**: Use the simplest code that works
- **Style consistency**: Follow Go conventions
- **Security**: No common security vulnerabilities
- **Performance**: Avoid obvious performance issues
- **Complexity limits**:
  - Functions: max 100 lines, 50 statements
  - Cyclomatic complexity: max 15
  - Cognitive complexity: max 20

### Code Style

- Use `gofmt` for formatting: `make fmt`
- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Write clear, self-documenting code
- Add comments for complex logic
- Use meaningful variable and function names

### Example Naming Conventions

When writing examples in documentation, DSL guides, or tests:

- **Use generic names** like `./src`, `./my-project`, `./app` for paths
- **Avoid internal/private project names** - don't reference real internal codebases
- **Use descriptive but neutral names** like `analyze_codebase`, `process_data`
- **Variable examples**: `$SRC_INDEX`, `$PROJECT_DATA`, `$RESULTS`

### Error Handling

```go
// Good
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad - don't ignore errors
result, _ := doSomething()
```

### Function Complexity

Keep functions focused and simple:

```go
// Good - single responsibility
func parseYAML(filename string) (*Config, error) {
    data, err := readFile(filename)
    if err != nil {
        return nil, err
    }
    return unmarshalConfig(data)
}

// Bad - too many responsibilities
func parseYAMLAndProcessAndSave(filename string) error {
    // 100+ lines of mixed concerns
}
```

## Testing Guidelines

### Unit Tests

- Write tests for all new functionality
- Aim for >80% code coverage
- Tests should be fast and independent
- Use table-driven tests when appropriate

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:  "valid input",
            input: "test",
            want:  "result",
        },
        {
            name:    "invalid input",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("MyFunction() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

- Add integration tests for new workflows
- Tests should handle missing API keys gracefully
- Document any prerequisites
- See [tests/integration/README.md](tests/integration/README.md)

### Running Tests

```bash
# Unit tests
make test

# With coverage report
make coverage

# Integration tests (requires API keys)
make integration

# All checks (lint + vet + test)
make check
```

## Submitting Changes

### Pre-submission Checklist

Before submitting your PR, ensure:

- [ ] Code passes all linting checks (`make lint-check`)
- [ ] All tests pass (`make test`)
- [ ] New functionality has tests
- [ ] Documentation is updated
- [ ] Commit messages are clear and descriptive
- [ ] Branch is up to date with main

### Creating a Pull Request

1. **Push your changes to your fork**
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create a Pull Request** on GitHub

3. **Fill out the PR template** with:
   - Clear description of changes
   - Motivation and context
   - Related issue number (if applicable)
   - Screenshots (for UI changes)
   - Checklist confirmation

4. **Respond to review feedback**
   - Address all comments
   - Push updates to the same branch
   - Re-request review when ready

### Commit Message Guidelines

Write clear, descriptive commit messages:

```
feat: add support for new LLM provider

- Implement provider interface
- Add configuration handling
- Include unit tests

Closes #123
```

Format: `type: subject`

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `chore`: Maintenance tasks

## Reporting Bugs

### Before Submitting a Bug Report

- Check the [existing issues](https://github.com/kris-hansen/comanda/issues)
- Verify you're using the latest version
- Try to reproduce with minimal configuration

### How to Submit a Bug Report

Create an issue with:

1. **Clear title** describing the problem
2. **Steps to reproduce** the issue
3. **Expected behavior** vs actual behavior
4. **Environment details**:
   - OS and version
   - Go version
   - comanda version
5. **Relevant logs** or error messages
6. **Example YAML** (if applicable)

## Suggesting Enhancements

We welcome feature suggestions! When proposing an enhancement:

1. **Check existing issues** for similar suggestions
2. **Describe the use case** - what problem does it solve?
3. **Explain the expected behavior**
4. **Consider alternatives** you've thought about
5. **Be open to discussion** about the implementation

## Development Workflow Summary

```bash
# 1. Setup
git checkout -b feature/amazing-feature
make deps

# 2. Develop
# ... make your changes ...
make lint          # Fix code style issues
make test          # Verify tests pass

# 3. Commit
git add .
git commit -m "feat: add amazing feature"

# 4. Pre-submit
make check         # Final checks
git push origin feature/amazing-feature

# 5. Create PR on GitHub
```

## Additional Resources

- [Project README](README.md)
- [Integration Tests Guide](tests/integration/README.md)
- [Example Workflows](examples/README.md)
- [Go Documentation](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go.html)

## Questions?

If you have questions about contributing:

- Check the [README](README.md) and existing documentation
- Look through [closed issues](https://github.com/kris-hansen/comanda/issues?q=is%3Aissue+is%3Aclosed)
- Open a new issue with the `question` label

Thank you for contributing to COMandA! 🚀
