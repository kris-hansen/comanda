package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var validateRuntimeDir string
var validateVarsFlags []string

var validateCmd = &cobra.Command{
	Use:   "validate <workflow.yaml> [additional workflows...]",
	Short: "Dry-run workflow validation without processing",
	Long: `Validate performs the same preflight checks used by processing without
running model actions, shell tools, quality gates, or writing workflow output.

It checks YAML structure, step dependencies, configured models/providers, input
files and URLs, database connectivity, tool allowlists, and output permissions.`,
	Example: `  comanda validate workflow.yaml
  comanda validate workflow.yaml --runtime-dir ./data
  comanda validate workflow.yaml --vars filename=./input.txt`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringVar(&validateRuntimeDir, "runtime-dir", "", "Runtime directory for file operations (relative to data directory)")
	validateCmd.Flags().StringArrayVar(&validateVarsFlags, "vars", []string{}, "Variable substitution in format key=value (can be repeated, use STDIN as value to map stdin)")
}

func runValidate(_ *cobra.Command, args []string) error {
	stdinData, err := readValidateStdin()
	if err != nil {
		return err
	}
	cliVars := parseVarsFlags(validateVarsFlags, stdinData)
	runtime := resolveProcessRuntimeDir(validateRuntimeDir)

	for _, file := range args {
		yamlFile, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read workflow %s: %w", file, err)
		}
		structure := processor.ValidateWorkflowStructure(string(yamlFile))
		if !structure.Valid {
			return fmt.Errorf("workflow %s is invalid:\n%s", file, structure.ErrorSummary())
		}

		var dslConfig processor.DSLConfig
		if err := yaml.Unmarshal(yamlFile, &dslConfig); err != nil {
			return fmt.Errorf("parse workflow %s: %w", file, err)
		}
		if err := validateComandaDirIfNeeded(&dslConfig); err != nil {
			return err
		}

		proc := processor.NewProcessor(&dslConfig, envConfig, &config.ServerConfig{Enabled: false}, verbose, runtime, cliVars)
		proc.SetWorkflowFile(file)
		if stdinData != "" {
			proc.SetLastOutput(stdinData)
		}
		if err := proc.Preflight(); err != nil {
			return fmt.Errorf("workflow %s cannot be processed: %w", file, err)
		}
		log.Printf("✓ %s can be processed (dry run; no workflow actions were executed)", file)
	}
	return nil
}

func readValidateStdin() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return "", nil
	}
	reader := bufio.NewReader(os.Stdin)
	var builder strings.Builder
	for {
		input, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return "", fmt.Errorf("read stdin: %w", readErr)
		}
		builder.WriteString(input)
		if readErr == io.EOF {
			break
		}
	}
	return builder.String(), nil
}

func validateComandaDirIfNeeded(dslConfig *processor.DSLConfig) error {
	if _, err := os.Stat(".comanda"); err == nil || !workflowNeedsComandaDir(dslConfig) {
		return nil
	}
	if err := preflightWritablePathForCommand(".comanda"); err != nil {
		return fmt.Errorf("workflow requires .comanda/ but it cannot be created: %w", err)
	}
	return nil
}

func workflowNeedsComandaDir(dslConfig *processor.DSLConfig) bool {
	for _, step := range dslConfig.Steps {
		if workflowUsesComandaPath(step.Config) {
			return true
		}
	}
	for _, loop := range dslConfig.Loops {
		for _, path := range loop.AllowedPaths {
			if strings.Contains(path, ".comanda") {
				return true
			}
		}
	}
	return false
}

func preflightWritablePathForCommand(path string) error {
	parent := "."
	if _, err := os.Stat(parent); err != nil {
		return err
	}
	probe, err := os.CreateTemp(parent, ".comanda-validate-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
