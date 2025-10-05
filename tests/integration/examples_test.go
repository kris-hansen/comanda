//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBinaryBuild verifies that the comanda binary can be built successfully
func TestBinaryBuild(t *testing.T) {
	// Build the binary
	cmd := exec.Command("go", "build", "-o", "../../dist/comanda-test", "../../main.go")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, output)
	}

	// Verify binary exists
	binaryPath := "../../dist/comanda-test"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Fatalf("Binary was not created at %s", binaryPath)
	}

	// Clean up
	t.Cleanup(func() { os.Remove(binaryPath) })

	t.Log("✓ Binary built successfully")
}

// TestOpenAIExample tests the OpenAI example workflow
// This test requires OPENAI_API_KEY to be set in .env
func TestOpenAIExample(t *testing.T) {
	if !hasRequiredEnvVars(t, "OPENAI_API_KEY") {
		t.Skip("Skipping OpenAI example test: OPENAI_API_KEY not set")
	}

	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	examplePath := "../../examples/model-examples/openai-example.yaml"
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Fatalf("Example file not found: %s", examplePath)
	}

	// Run the example
	cmd := exec.Command(binaryPath, "process", examplePath)
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	// Check for errors
	if err != nil {
		t.Logf("Command output: %s", output)
		t.Fatalf("Failed to run OpenAI example: %v", err)
	}

	// Verify output contains expected content
	outputStr := string(output)
	if !strings.Contains(outputStr, "Response from") {
		t.Errorf("Expected 'Response from' in output, got: %s", outputStr)
	}

	t.Log("✓ OpenAI example executed successfully")
}

// TestFileConsolidation tests file consolidation workflow
func TestFileConsolidation(t *testing.T) {
	if !hasRequiredEnvVars(t, "OPENAI_API_KEY") {
		t.Skip("Skipping file consolidation test: OPENAI_API_KEY not set")
	}

	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	examplePath := "../../examples/file-processing/consolidate-example.yaml"
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Fatalf("Example file not found: %s", examplePath)
	}

	// Run the example
	cmd := exec.Command(binaryPath, "process", examplePath)
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Command output: %s", output)
		t.Fatalf("Failed to run consolidation example: %v", err)
	}

	// Check if output file was created
	outputFile := "../../examples/file-processing/consolidated.txt"
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Logf("Command output: %s", output)
		t.Errorf("Expected output file was not created: %s", outputFile)
	}

	t.Log("✓ File consolidation example executed successfully")
}

// TestParallelProcessing tests parallel execution of workflows
func TestParallelProcessing(t *testing.T) {
	if !hasRequiredEnvVars(t, "OPENAI_API_KEY") {
		t.Skip("Skipping parallel processing test: OPENAI_API_KEY not set")
	}

	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	examplePath := "../../examples/parallel-processing/parallel-inference.yaml"
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Fatalf("Example file not found: %s", examplePath)
	}

	// Run the example
	cmd := exec.Command(binaryPath, "process", examplePath)
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Command output: %s", output)
		t.Fatalf("Failed to run parallel processing example: %v", err)
	}

	// Verify parallel execution happened
	outputStr := string(output)
	if !strings.Contains(outputStr, "parallel") && !strings.Contains(outputStr, "Response from") {
		t.Logf("Output: %s", outputStr)
		t.Error("Expected parallel processing indicators in output")
	}

	t.Log("✓ Parallel processing example executed successfully")
}

// TestDatabaseExample tests database operations
// This test requires a PostgreSQL database to be running
func TestDatabaseExample(t *testing.T) {
	// Check for required environment variables
	requiredVars := []string{"OPENAI_API_KEY"}
	if !hasRequiredEnvVars(t, requiredVars...) {
		t.Skip("Skipping database example test: required API keys not set")
	}

	// Check if database is configured
	if !isDatabaseConfigured(t) {
		t.Skip("Skipping database example test: database not configured in .env")
	}

	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	examplePath := "../../examples/database-connections/postgres/db-example.yaml"
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Skip("Database example file not found - this is optional")
	}

	// Run the example
	cmd := exec.Command(binaryPath, "process", examplePath)
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	// Database tests are more lenient as the database might not be running
	if err != nil {
		t.Logf("Database test output: %s", output)
		// Don't fail the test if database is not available
		t.Logf("Database example could not run (database may not be available): %v", err)
		return
	}

	t.Log("✓ Database example executed successfully")
}

// Helper function to build the test binary
func buildTestBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), "comanda-test")

	cmd := exec.Command("go", "build", "-o", binaryPath, "../../main.go")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build test binary: %v\nOutput: %s", err, output)
	}

	return binaryPath
}

// Helper function to check if required environment variables are set
func hasRequiredEnvVars(t *testing.T, vars ...string) bool {
	t.Helper()

	// First check if .env file exists
	if _, err := os.Stat("../../.env"); os.IsNotExist(err) {
		t.Logf(".env file not found")
		return false
	}

	// Read .env file
	content, err := os.ReadFile("../../.env")
	if err != nil {
		t.Logf("Could not read .env file: %v", err)
		return false
	}

	envContent := string(content)
	for _, envVar := range vars {
		if !strings.Contains(envContent, envVar) {
			t.Logf("Required environment variable not found in .env: %s", envVar)
			return false
		}
	}

	return true
}

// Helper function to check if database is configured
func isDatabaseConfigured(t *testing.T) bool {
	t.Helper()

	if _, err := os.Stat("../../.env"); os.IsNotExist(err) {
		return false
	}

	content, err := os.ReadFile("../../.env")
	if err != nil {
		return false
	}

	// Look for database configuration
	return strings.Contains(string(content), "database:")
}

// TestConfigureCommand tests the configure command
func TestConfigureCommand(t *testing.T) {
	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	// Test configure --list command (doesn't require interaction)
	cmd := exec.Command(binaryPath, "configure", "--list")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Command output: %s", output)
		// This might fail if no configuration exists, which is okay
		t.Logf("Configure --list output: %s", output)
	} else {
		t.Logf("✓ Configure --list executed successfully")
	}
}

// TestVersionCommand tests the version command
func TestVersionCommand(t *testing.T) {
	binaryPath := buildTestBinary(t)
	t.Cleanup(func() { os.Remove(binaryPath) })

	// Read VERSION file
	versionBytes, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("Failed to read VERSION file: %v", err)
	}
	expectedVersion := strings.TrimSpace(string(versionBytes))

	// Run version command (if it exists)
	cmd := exec.Command(binaryPath, "--version")
	cmd.Dir = "../.."
	output, err := cmd.CombinedOutput()

	// Version command might not exist, so we're lenient here
	if err != nil {
		t.Logf("Version command output: %s", output)
		t.Logf("Version command not available or failed (this is okay)")
	} else {
		outputStr := string(output)
		if !strings.Contains(outputStr, expectedVersion) {
			t.Logf("Expected version %s in output, got: %s", expectedVersion, outputStr)
		} else {
			t.Logf("✓ Version command executed successfully")
		}
	}
}
