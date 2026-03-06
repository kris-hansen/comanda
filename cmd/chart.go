package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/kris-hansen/comanda/utils/tui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Chart box styles using lipgloss
var (
	chartBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	chartDoubleBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)

	chartHeaderStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color("99")).
				Padding(0, 1).
				Bold(true)
)

// ChartNode represents a step in the workflow visualization
type ChartNode struct {
	Name        string
	Model       string
	Action      string
	Input       []string
	Output      []string
	IsParallel  bool
	ParallelID  string
	StepType    string // "standard", "generate", "process", "openai-responses"
	IsValid     bool
	InvalidMsg  string
	DependsOn   []string // Names of steps this depends on
	ProducesFor []string // Names of steps that depend on this
}

// WorkflowChart contains the parsed workflow structure for visualization
type WorkflowChart struct {
	Nodes           []ChartNode
	ParallelGroups  map[string][]ChartNode
	AgenticLoopCfgs map[string]*processor.AgenticLoopConfig // standalone agentic-loop blocks
	DeferredSteps   []ChartNode
	EntryInputs     []string
	FinalOutputs    []string
	SequentialOrder []string
	HasStdinEntry   bool
	HasToolInputs   bool
	HasErrors       bool
	Errors          []string
}

var (
	chartFormat      string
	chartInteractive bool
)

var chartCmd = &cobra.Command{
	Use:   "chart <workflow.yaml>",
	Short: "Display a chart visualization of a workflow",
	Long: `Display a visual representation of a workflow file showing:
  - Relationship between steps (sequential and parallel)
  - Step names, model names, and brief action summaries
  - Validity of each step
  - Input/output chains between steps
  - Workflow statistics

Output formats:
  - ascii (default): Unicode box-drawing chart for terminal
  - mermaid: Mermaid flowchart syntax for docs/README`,
	Example: `  # Display workflow chart (ASCII)
  comanda chart workflow.yaml

  # Display as Mermaid flowchart
  comanda chart workflow.yaml --format mermaid

  # Display chart with verbose output
  comanda chart workflow.yaml --verbose`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowFile := args[0]

		// Read workflow file
		yamlFile, err := os.ReadFile(workflowFile)
		if err != nil {
			return fmt.Errorf("error reading workflow file: %w", err)
		}

		// Parse workflow
		var dslConfig processor.DSLConfig
		if err := yaml.Unmarshal(yamlFile, &dslConfig); err != nil {
			return fmt.Errorf("error parsing workflow: %w", err)
		}

		// Build chart structure
		chart := buildWorkflowChart(&dslConfig)

		// Render based on format/mode
		if chartInteractive {
			return renderInteractiveChart(chart, workflowFile)
		}

		switch chartFormat {
		case "mermaid":
			renderMermaidChart(chart, workflowFile)
		default:
			renderChart(chart, workflowFile)
		}

		return nil
	},
}

func init() {
	chartCmd.Flags().StringVarP(&chartFormat, "format", "f", "ascii", "Output format: ascii, mermaid")
	chartCmd.Flags().BoolVarP(&chartInteractive, "interactive", "i", false, "Interactive TUI mode with colors and navigation")
	rootCmd.AddCommand(chartCmd)
}

// buildWorkflowChart parses the DSLConfig and builds a WorkflowChart
func buildWorkflowChart(config *processor.DSLConfig) *WorkflowChart {
	chart := &WorkflowChart{
		Nodes:           []ChartNode{},
		ParallelGroups:  make(map[string][]ChartNode),
		AgenticLoopCfgs: make(map[string]*processor.AgenticLoopConfig),
		DeferredSteps:   []ChartNode{},
	}

	// Track output producers for dependency detection
	outputProducers := make(map[string]string) // output file -> step name

	// Track all inputs and outputs for entry/exit detection
	allInputs := make(map[string]bool)
	allOutputs := make(map[string]bool)
	hasStdout := false

	// Check for multi-loop orchestration
	if len(config.Loops) > 0 {
		processMultiLoopWorkflow(config, chart)
		return chart
	}

	// Check for standalone agentic-loop blocks
	if len(config.AgenticLoops) > 0 {
		processAgenticLoopBlocks(config, chart)
		return chart
	}

	// Process parallel steps first (with nil check)
	if config.ParallelSteps != nil {
		for groupName, steps := range config.ParallelSteps {
			var groupNodes []ChartNode
			for _, step := range steps {
				node := stepToChartNode(step, true, groupName)
				groupNodes = append(groupNodes, node)

				// Track outputs
				for _, out := range node.Output {
					if out == "STDOUT" {
						hasStdout = true
					} else if out != "MEMORY" {
						outputProducers[out] = node.Name
						allOutputs[out] = true
					}
				}
				// Track inputs
				for _, in := range node.Input {
					if in == "STDIN" {
						chart.HasStdinEntry = true
					} else if strings.HasPrefix(in, "tool:") {
						chart.HasToolInputs = true
					} else if in != "NA" {
						allInputs[in] = true
					}
				}
			}
			chart.ParallelGroups[groupName] = groupNodes
			chart.SequentialOrder = append(chart.SequentialOrder, "parallel:"+groupName)
		}
	}

	// Process sequential steps (with nil check)
	if config.Steps != nil {
		for _, step := range config.Steps {
			node := stepToChartNode(step, false, "")
			chart.Nodes = append(chart.Nodes, node)
			chart.SequentialOrder = append(chart.SequentialOrder, node.Name)

			// Track outputs
			for _, out := range node.Output {
				if out == "STDOUT" {
					hasStdout = true
				} else if out != "MEMORY" {
					outputProducers[out] = node.Name
					allOutputs[out] = true
				}
			}
			// Track inputs
			for _, in := range node.Input {
				if in == "STDIN" {
					chart.HasStdinEntry = true
				} else if strings.HasPrefix(in, "tool:") {
					chart.HasToolInputs = true
				} else if in != "NA" {
					allInputs[in] = true
				}
			}
		}
	}

	// Process deferred steps (with nil check)
	if config.Defer != nil {
		for stepName, stepConfig := range config.Defer {
			step := processor.Step{Name: stepName, Config: stepConfig}
			node := stepToChartNode(step, false, "")
			node.StepType = "deferred"
			chart.DeferredSteps = append(chart.DeferredSteps, node)
		}
	}

	// Build dependencies
	buildDependencies(chart, outputProducers)

	// Determine entry inputs (inputs that are not produced by any step)
	// Exclude tool: inputs from entry points (they are generated at runtime)
	for input := range allInputs {
		if _, isProduced := outputProducers[input]; !isProduced {
			// Filter out tool: prefixed inputs
			if !strings.HasPrefix(input, "tool:") {
				chart.EntryInputs = append(chart.EntryInputs, input)
			}
		}
	}
	sort.Strings(chart.EntryInputs)

	// Determine final outputs (outputs that are not consumed by any step)
	consumedOutputs := make(map[string]bool)
	for _, node := range chart.Nodes {
		for _, in := range node.Input {
			consumedOutputs[in] = true
		}
	}
	for _, groupNodes := range chart.ParallelGroups {
		for _, node := range groupNodes {
			for _, in := range node.Input {
				consumedOutputs[in] = true
			}
		}
	}

	for output := range allOutputs {
		if !consumedOutputs[output] {
			chart.FinalOutputs = append(chart.FinalOutputs, output)
		}
	}
	sort.Strings(chart.FinalOutputs)

	// Add STDOUT as final output only once if any step outputs to it
	if hasStdout {
		chart.FinalOutputs = append(chart.FinalOutputs, "STDOUT")
	}

	return chart
}

// stepToChartNode converts a processor.Step to a ChartNode
func stepToChartNode(step processor.Step, isParallel bool, parallelID string) ChartNode {
	node := ChartNode{
		Name:       step.Name,
		IsParallel: isParallel,
		ParallelID: parallelID,
		IsValid:    true,
	}

	// Determine step type
	if step.Config.Generate != nil {
		node.StepType = "generate"
		node.Model = normalizeValue(step.Config.Generate.Model)
		node.Action = truncateAction(normalizeValue(step.Config.Generate.Action))
		node.Output = []string{step.Config.Generate.Output}
	} else if step.Config.Process != nil {
		node.StepType = "process"
		node.Model = modelNA
		node.Action = fmt.Sprintf("Process: %s", step.Config.Process.WorkflowFile)
	} else if step.Config.Type == "openai-responses" {
		node.StepType = "openai-responses"
		node.Model = normalizeValue(step.Config.Model)
		node.Action = truncateAction(step.Config.Instructions)
	} else {
		node.StepType = "standard"
		node.Model = normalizeValue(step.Config.Model)
		node.Action = truncateAction(normalizeValue(step.Config.Action))
	}

	// Parse inputs
	node.Input = normalizeStringSlice(step.Config.Input)

	// Parse outputs (if not already set by generate step)
	if len(node.Output) == 0 {
		node.Output = normalizeStringSlice(step.Config.Output)
	}

	// Validate the node
	validateNode(&node, step.Config)

	return node
}

// validateNode checks if a step configuration is valid
func validateNode(node *ChartNode, config processor.StepConfig) {
	var errors []string

	switch node.StepType {
	case "standard":
		if len(node.Input) == 0 {
			errors = append(errors, "missing input")
		}
		if node.Model == "" || node.Model == "[]" {
			errors = append(errors, "missing model")
		}
		if node.Action == "" || node.Action == "[]" {
			errors = append(errors, "missing action")
		}
		if len(node.Output) == 0 {
			errors = append(errors, "missing output")
		}
	case "generate":
		if node.Action == "" {
			errors = append(errors, "missing action")
		}
		if len(node.Output) == 0 || node.Output[0] == "" {
			errors = append(errors, "missing output filename")
		}
	case "process":
		if config.Process != nil && config.Process.WorkflowFile == "" {
			errors = append(errors, "missing workflow_file")
		}
	}

	if len(errors) > 0 {
		node.IsValid = false
		node.InvalidMsg = strings.Join(errors, ", ")
	}
}

// buildDependencies populates the DependsOn and ProducesFor fields
func buildDependencies(chart *WorkflowChart, outputProducers map[string]string) {
	// Build dependency graph for sequential nodes
	for i := range chart.Nodes {
		node := &chart.Nodes[i]
		for _, input := range node.Input {
			if producer, exists := outputProducers[input]; exists {
				node.DependsOn = append(node.DependsOn, producer)
			}
		}
		// Check for STDIN dependency on previous step
		for _, input := range node.Input {
			if input == "STDIN" && i > 0 {
				// Depends on whatever came before (could be parallel group or sequential step)
				// This is a simplification - in practice STDIN chains are more complex
				break
			}
		}
	}

	// Build ProducesFor (reverse of DependsOn)
	producerToConsumers := make(map[string][]string)
	for _, node := range chart.Nodes {
		for _, dep := range node.DependsOn {
			producerToConsumers[dep] = append(producerToConsumers[dep], node.Name)
		}
	}
	for i := range chart.Nodes {
		if consumers, exists := producerToConsumers[chart.Nodes[i].Name]; exists {
			chart.Nodes[i].ProducesFor = consumers
		}
	}

	// Same for parallel nodes
	for groupName, groupNodes := range chart.ParallelGroups {
		for i := range groupNodes {
			node := &groupNodes[i]
			for _, input := range node.Input {
				if producer, exists := outputProducers[input]; exists {
					node.DependsOn = append(node.DependsOn, producer)
				}
			}
		}
		chart.ParallelGroups[groupName] = groupNodes
	}
}

// renderChart outputs the ASCII visualization of the workflow
func renderChart(chart *WorkflowChart, filename string) {
	const boxWidth = 50

	// Header
	fmt.Println()
	printBox("WORKFLOW: "+filename, boxWidth)
	fmt.Println()

	// Entry point box
	entryText := getEntryPointText(chart)
	if entryText != "" {
		printSmallBox(entryText, boxWidth)
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + boxVert)
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + arrowDown)
	}

	// Render steps in order
	if len(chart.SequentialOrder) == 0 {
		fmt.Println("  (no steps defined)")
	}
	stepNum := 0
	totalItems := len(chart.SequentialOrder)
	for idx, item := range chart.SequentialOrder {
		isLast := idx == totalItems-1
		if strings.HasPrefix(item, "parallel:") {
			groupName := strings.TrimPrefix(item, "parallel:")
			stepNum++
			renderParallelGroup(chart.ParallelGroups[groupName], groupName, stepNum, boxWidth)
		} else if strings.HasPrefix(item, "agentic-loop:") {
			loopKey := strings.TrimPrefix(item, "agentic-loop:")
			stepNum++
			if loopCfg, ok := chart.AgenticLoopCfgs[loopKey]; ok {
				renderAgenticLoopBox(loopCfg, boxWidth)
			}
		} else {
			for _, node := range chart.Nodes {
				if node.Name == item {
					stepNum++
					renderNodeBox(node, stepNum, boxWidth)
					break
				}
			}
		}
		// Arrow between steps
		if !isLast {
			fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + boxVert)
			fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + arrowDown)
		}
	}

	// Exit point box
	fmt.Println()
	exitText := getExitPointText(chart)
	if exitText != "" {
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + boxVert)
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + arrowDown)
		printSmallBox(exitText, boxWidth)
	}

	// Render deferred steps if any
	if len(chart.DeferredSteps) > 0 {
		fmt.Println()
		fmt.Println("  DEFERRED (conditional):")
		for _, node := range chart.DeferredSteps {
			summary := summarizeAction(node.Action, node.StepType)
			fmt.Printf("    ? %s [%s] - %s\n", node.Name, node.Model, summary)
		}
	}

	// Statistics
	fmt.Println()
	printStatsBox(chart, boxWidth)
}

// Unicode box-drawing characters
// Chart icons and connectors (boxes now rendered via lipgloss)
const (
	boxVert      = "│" // Used for flow connectors
	arrowDown    = "▼" // Used for flow connectors
	loopIcon     = "*"
	parallelIcon = "="
	modelNA      = "N/A" // Placeholder for missing model
)

// printBox prints a double-line box with centered text
func printBox(text string, width int) {
	style := chartDoubleBoxStyle.Padding(0, 4).Align(lipgloss.Center)
	fmt.Println(style.Render(text))
}

// printSmallBox prints a single-line box
func printSmallBox(text string, width int) {
	style := chartBoxStyle.Padding(0, 4).Align(lipgloss.Center)
	fmt.Println(style.Render(text))
}

// printStepBox prints a step box with name, model, and summary
func printStepBox(name, model, summary string, isValid bool, width int, node *ChartNode) {
	status := "✓"
	if !isValid {
		status = "✗"
	}

	// Build content lines
	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s", status, name))

	// Add model if present
	if model != "" && model != modelNA && model != "[]" {
		lines = append(lines, fmt.Sprintf("Model: %s", model))
	}

	// Add action/summary if present
	if summary != "" {
		lines = append(lines, summary)
	}

	// Add output if available
	if node != nil && len(node.Output) > 0 {
		outStr := strings.Join(node.Output, ", ")
		if outStr != "" && outStr != "STDOUT" && outStr != "[]" {
			lines = append(lines, fmt.Sprintf("Output: %s", outStr))
		}
	}

	// Render with lipgloss
	fmt.Println(tui.RenderMultiLineBox(lines, width, false))
}

// printStatsBox prints the statistics in a box
func printStatsBox(chart *WorkflowChart, width int) {
	totalSteps := len(chart.Nodes)
	parallelSteps := 0
	for _, nodes := range chart.ParallelGroups {
		parallelSteps += len(nodes)
	}
	totalSteps += parallelSteps

	validSteps := 0
	loopCount := 0
	for _, node := range chart.Nodes {
		if node.IsValid {
			validSteps++
		}
		if node.StepType == "agentic-loop" {
			loopCount++
		}
	}
	for _, nodes := range chart.ParallelGroups {
		for _, node := range nodes {
			if node.IsValid {
				validSteps++
			}
		}
	}

	// Count models
	modelCounts := make(map[string]int)
	for _, node := range chart.Nodes {
		if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
			modelCounts[node.Model]++
		}
	}
	for _, nodes := range chart.ParallelGroups {
		for _, node := range nodes {
			if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
				modelCounts[node.Model]++
			}
		}
	}

	// Build content lines
	var lines []string
	lines = append(lines, "STATISTICS")

	if loopCount > 0 {
		lines = append(lines, fmt.Sprintf("%s Loops: %d agentic", loopIcon, loopCount))
	} else {
		lines = append(lines, fmt.Sprintf("Steps: %d total, %d parallel", totalSteps, parallelSteps))
	}
	if len(chart.DeferredSteps) > 0 {
		lines = append(lines, fmt.Sprintf("Deferred: %d conditional", len(chart.DeferredSteps)))
	}
	if loopCount == 0 {
		lines = append(lines, fmt.Sprintf("Valid: %d/%d", validSteps, totalSteps))
	}

	if len(modelCounts) > 0 {
		var models []string
		for model := range modelCounts {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			lines = append(lines, fmt.Sprintf("%s (%d)", model, modelCounts[model]))
		}
	}

	fmt.Println(tui.RenderMultiLineBox(lines, width, true))
}

// getEntryPointText returns a short description of entry points
func getEntryPointText(chart *WorkflowChart) string {
	if chart.HasToolInputs {
		return "INPUT: Tool execution"
	}
	if chart.HasStdinEntry && len(chart.EntryInputs) == 0 {
		return "INPUT: STDIN"
	}
	if len(chart.EntryInputs) == 1 {
		input := chart.EntryInputs[0]
		if len(input) > 30 {
			input = "..." + input[len(input)-27:]
		}
		return "INPUT: " + input
	}
	if len(chart.EntryInputs) > 1 {
		return fmt.Sprintf("INPUT: %d files", len(chart.EntryInputs))
	}
	// Check for NA input
	if len(chart.Nodes) > 0 {
		firstNode := chart.Nodes[0]
		if len(firstNode.Input) == 1 && firstNode.Input[0] == "NA" {
			return "INPUT: None required"
		}
	}
	return ""
}

// getExitPointText returns a short description of exit points
func getExitPointText(chart *WorkflowChart) string {
	if len(chart.FinalOutputs) == 0 {
		return ""
	}
	hasStdout := false
	fileCount := 0
	var lastFile string
	for _, out := range chart.FinalOutputs {
		if out == "STDOUT" {
			hasStdout = true
		} else {
			fileCount++
			lastFile = out
		}
	}

	if hasStdout && fileCount == 0 {
		return "OUTPUT: STDOUT"
	}
	if !hasStdout && fileCount == 1 {
		if len(lastFile) > 30 {
			lastFile = "..." + lastFile[len(lastFile)-27:]
		}
		return "OUTPUT: " + lastFile
	}
	if hasStdout && fileCount > 0 {
		return fmt.Sprintf("OUTPUT: STDOUT + %d files", fileCount)
	}
	return fmt.Sprintf("OUTPUT: %d files", fileCount)
}

// renderNodeBox draws a step as an ASCII box
func renderNodeBox(node ChartNode, stepNum int, boxWidth int) {
	summary := summarizeAction(node.Action, node.StepType)
	printStepBox(node.Name, node.Model, summary, node.IsValid, boxWidth, &node)
}

// summarizeAction creates a brief summary of the action (up to 5 words)
func summarizeAction(action string, stepType string) string {
	if action == "" {
		return ""
	}

	// Handle agentic-loop step type - already formatted
	if stepType == "agentic-loop" {
		return action
	}

	// Handle process step type - already formatted
	if stepType == "process" {
		return action
	}

	// Clean up the action
	action = strings.ReplaceAll(action, "\n", " ")
	action = strings.Join(strings.Fields(action), " ")

	// For generate steps, try to find what's being generated
	if stepType == "generate" {
		lower := strings.ToLower(action)
		// Look for "create a/an X" or "generate a/an X" patterns
		for _, prefix := range []string{"create a ", "create an ", "generate a ", "generate an ", "write a ", "write an "} {
			if idx := strings.Index(lower, prefix); idx != -1 {
				rest := action[idx+len(prefix):]
				words := strings.Fields(rest)
				if len(words) >= 3 {
					return "Generate " + strings.Join(words[:3], " ")
				}
				if len(words) >= 2 {
					return "Generate " + strings.Join(words[:2], " ")
				}
				if len(words) >= 1 {
					return "Generate " + words[0]
				}
			}
		}
		return "Generate workflow"
	}

	lower := strings.ToLower(action)

	// Extract key action words with context
	keywords := []string{
		"analyze", "summarize", "extract", "create", "generate", "translate",
		"compare", "review", "convert", "format", "validate",
		"parse", "filter", "transform", "merge", "split", "combine",
		"identify", "classify", "categorize", "sort", "search", "find",
		"explain", "describe", "report", "list", "count", "calculate",
		"suggest", "recommend", "evaluate", "assess", "check", "verify",
	}

	for _, kw := range keywords {
		if idx := strings.Index(lower, kw); idx != -1 {
			// Get the keyword and up to 4 more words (5 total)
			rest := action[idx:]
			words := strings.Fields(rest)
			numWords := 5
			if len(words) < numWords {
				numWords = len(words)
			}
			if numWords > 0 {
				result := strings.Join(words[:numWords], " ")
				// Capitalize first letter
				if len(result) > 0 {
					return strings.ToUpper(string(result[0])) + result[1:]
				}
			}
			return strings.ToUpper(string(kw[0])) + kw[1:]
		}
	}

	// Fallback: take first 5 words or mark as complex
	words := strings.Fields(action)
	if len(words) > 20 {
		// Very long prompt, likely complex
		return "Complex prompt"
	}
	if len(words) >= 5 {
		return strings.Join(words[:5], " ")
	}
	if len(words) > 0 {
		return strings.Join(words, " ")
	}
	return "Process data"
}

// renderNodeInline draws a node inline for parallel display
func renderNodeInline(node ChartNode, width int) {
	status := "✓"
	if !node.IsValid {
		status = "✗"
	}
	summary := summarizeAction(node.Action, node.StepType)

	// Build content lines
	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s", status, node.Name))

	if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
		lines = append(lines, fmt.Sprintf("Model: %s", node.Model))
	}

	if summary != "" {
		lines = append(lines, summary)
	}

	if len(node.Output) > 0 {
		outStr := strings.Join(node.Output, ", ")
		if outStr != "" && outStr != "STDOUT" && outStr != "[]" {
			lines = append(lines, fmt.Sprintf("Output: %s", outStr))
		}
	}

	// Render with indent
	box := tui.RenderMultiLineBox(lines, width-4, false)
	for _, line := range strings.Split(box, "\n") {
		fmt.Println("  " + line)
	}
}

// renderParallelGroup draws a parallel execution group
func renderParallelGroup(nodes []ChartNode, groupName string, stepNum int, boxWidth int) {
	// Build header
	header := fmt.Sprintf("%s PARALLEL: %s (%d steps)", parallelIcon, groupName, len(nodes))

	// Build content with inline nodes
	var contentLines []string
	for i, node := range nodes {
		status := "✓"
		if !node.IsValid {
			status = "✗"
		}
		contentLines = append(contentLines, fmt.Sprintf("  %s %s", status, node.Name))
		if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
			contentLines = append(contentLines, fmt.Sprintf("    Model: %s", node.Model))
		}
		if i < len(nodes)-1 {
			contentLines = append(contentLines, "  ┄┄┄")
		}
	}

	// Combine header and content
	allLines := append([]string{header}, contentLines...)
	fmt.Println(tui.RenderMultiLineBox(allLines, boxWidth, true))
}

// processAgenticLoopBlocks handles standalone agentic-loop blocks (config.AgenticLoops)
func processAgenticLoopBlocks(config *processor.DSLConfig, chart *WorkflowChart) {
	for loopName, loopConfig := range config.AgenticLoops {
		chart.AgenticLoopCfgs[loopName] = loopConfig
		chart.SequentialOrder = append(chart.SequentialOrder, "agentic-loop:"+loopName)

		// Count sub-steps as nodes for stats
		for _, step := range loopConfig.Steps {
			node := stepToChartNode(step, false, "")
			node.StepType = "agentic-loop"
			chart.Nodes = append(chart.Nodes, node)
		}
	}
}

// renderAgenticLoopBox draws a standalone agentic loop container
func renderAgenticLoopBox(config *processor.AgenticLoopConfig, boxWidth int) {
	displayName := "agentic-loop"
	if config.Name != "" {
		displayName = config.Name
	}

	// Build content lines
	var lines []string

	// Header
	lines = append(lines, fmt.Sprintf("%s %s", loopIcon, displayName))

	// Config line
	exitInfo := config.ExitCondition
	if exitInfo == "" {
		exitInfo = "llm_decides"
	}
	maxIter := config.MaxIterations
	if maxIter == 0 {
		maxIter = 10
	}
	configLine := fmt.Sprintf("Iterations: %d │ Exit: %s", maxIter, exitInfo)
	if config.TimeoutSeconds > 0 {
		configLine += fmt.Sprintf(" │ Timeout: %ds", config.TimeoutSeconds)
	}
	if config.Stateful {
		configLine += " │ Stateful"
	}
	lines = append(lines, configLine)

	if config.ContextWindow > 0 {
		lines = append(lines, fmt.Sprintf("Context Window: %d iterations", config.ContextWindow))
	}

	// Show allowed paths if any
	if len(config.AllowedPaths) > 0 {
		pathsPreview := strings.Join(config.AllowedPaths, ", ")
		lines = append(lines, fmt.Sprintf("Paths: %s", pathsPreview))
	}

	// Add steps
	lines = append(lines, "─────────────────────────")
	for i, step := range config.Steps {
		node := stepToChartNode(step, false, "")
		status := "✓"
		if !node.IsValid {
			status = "✗"
		}
		lines = append(lines, fmt.Sprintf("  %s %s", status, node.Name))
		if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
			lines = append(lines, fmt.Sprintf("    Model: %s", node.Model))
		}
		if i < len(config.Steps)-1 {
			lines = append(lines, "    ↓")
		}
	}

	// Loop-back indicator
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("↺ loop back (max %d iterations)", maxIter))

	fmt.Println(tui.RenderMultiLineBox(lines, boxWidth, true))
}

// processMultiLoopWorkflow handles multi-loop orchestration syntax
func processMultiLoopWorkflow(config *processor.DSLConfig, chart *WorkflowChart) {
	// Convert loops to chart nodes
	for loopName, loopConfig := range config.Loops {
		node := ChartNode{
			Name:       loopName,
			Model:      "claude-code", // Default for agentic loops
			StepType:   "agentic-loop",
			IsValid:    true,
			IsParallel: false,
		}

		// Build action summary from loop config
		actionParts := []string{fmt.Sprintf("Loop: %d iterations", loopConfig.MaxIterations)}
		if loopConfig.Stateful {
			actionParts = append(actionParts, "stateful")
		}
		if len(loopConfig.QualityGates) > 0 {
			actionParts = append(actionParts, fmt.Sprintf("%d gates", len(loopConfig.QualityGates)))
		}
		if loopConfig.TimeoutSeconds == 0 {
			actionParts = append(actionParts, "unlimited time")
		}
		node.Action = strings.Join(actionParts, ", ")

		// Add dependencies from depends_on
		if len(loopConfig.DependsOn) > 0 {
			node.DependsOn = loopConfig.DependsOn
		}

		// Track input/output state variables
		if loopConfig.InputState != "" {
			node.Input = []string{loopConfig.InputState}
		} else {
			node.Input = []string{"NA"}
		}
		if loopConfig.OutputState != "" {
			node.Output = []string{loopConfig.OutputState}
		} else {
			node.Output = []string{"STDOUT"}
		}

		chart.Nodes = append(chart.Nodes, node)
	}

	// Determine execution order
	if len(config.ExecuteLoops) > 0 {
		// Use execute_loops order
		chart.SequentialOrder = config.ExecuteLoops
	} else if len(config.Workflow) > 0 {
		// Use workflow order (creator/checker pattern)
		// Map workflow nodes to loop names
		for nodeName, workflowNode := range config.Workflow {
			if workflowNode.Loop != "" {
				chart.SequentialOrder = append(chart.SequentialOrder, workflowNode.Loop)
			} else {
				chart.SequentialOrder = append(chart.SequentialOrder, nodeName)
			}
		}
		sort.Strings(chart.SequentialOrder) // Stable order for display
	} else {
		// Topological sort based on dependencies
		chart.SequentialOrder = topologicalSort(chart.Nodes)
	}

	// Build ProducesFor relationships
	buildLoopProducesFor(chart)
}

// topologicalSort orders loop nodes by dependencies
func topologicalSort(nodes []ChartNode) []string {
	// Build in-degree map
	inDegree := make(map[string]int)

	for _, node := range nodes {
		if _, exists := inDegree[node.Name]; !exists {
			inDegree[node.Name] = 0
		}
		for range node.DependsOn {
			inDegree[node.Name]++
		}
	}

	// Find nodes with no dependencies
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue) // Stable order

	// Process queue
	var result []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Reduce in-degree for dependent nodes
		for _, node := range nodes {
			for _, dep := range node.DependsOn {
				if dep == current {
					inDegree[node.Name]--
					if inDegree[node.Name] == 0 {
						queue = append(queue, node.Name)
						sort.Strings(queue) // Keep stable
					}
				}
			}
		}
	}

	return result
}

// buildLoopProducesFor builds the ProducesFor relationships for loops
func buildLoopProducesFor(chart *WorkflowChart) {
	producerToConsumers := make(map[string][]string)
	for _, node := range chart.Nodes {
		for _, dep := range node.DependsOn {
			producerToConsumers[dep] = append(producerToConsumers[dep], node.Name)
		}
	}
	for i := range chart.Nodes {
		if consumers, exists := producerToConsumers[chart.Nodes[i].Name]; exists {
			chart.Nodes[i].ProducesFor = consumers
		}
	}
}

// Helper functions

func normalizeValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		if len(val) == 0 {
			return "[]"
		}
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = fmt.Sprintf("%v", item)
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return strings.Join(parts, ", ")
	case []string:
		if len(val) == 0 {
			return "[]"
		}
		if len(val) == 1 {
			return val[0]
		}
		return strings.Join(val, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func normalizeStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" || val == "NA" {
			return []string{val}
		}
		// Clean up multi-line strings for display
		cleaned := strings.ReplaceAll(val, "\n", " ")
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		return []string{cleaned}
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			str := fmt.Sprintf("%v", item)
			// Clean up multi-line strings
			str = strings.ReplaceAll(str, "\n", " ")
			str = strings.Join(strings.Fields(str), " ")
			result[i] = str
		}
		return result
	case []string:
		result := make([]string, len(val))
		for i, str := range val {
			// Clean up multi-line strings
			str = strings.ReplaceAll(str, "\n", " ")
			str = strings.Join(strings.Fields(str), " ")
			result[i] = str
		}
		return result
	default:
		str := fmt.Sprintf("%v", v)
		str = strings.ReplaceAll(str, "\n", " ")
		str = strings.Join(strings.Fields(str), " ")
		return []string{str}
	}
}

func truncateAction(action string) string {
	if action == "" {
		return ""
	}
	// Remove newlines and extra whitespace
	action = strings.ReplaceAll(action, "\n", " ")
	action = strings.Join(strings.Fields(action), " ")

	maxLen := 50
	if len(action) > maxLen {
		return action[:maxLen-3] + "..."
	}
	return action
}

func formatIOList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}

	// For multiple items, show first and count
	if len(items) <= 3 {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, %s, ... (+%d more)", items[0], items[1], len(items)-2)
}

// renderInteractiveChart displays an interactive TUI chart
func renderInteractiveChart(chart *WorkflowChart, filename string) error {
	// Convert ChartNodes to TUI nodes
	var tuiNodes []*tui.ChartNode

	// Add regular nodes
	for _, node := range chart.Nodes {
		tuiNode := &tui.ChartNode{
			Name:    node.Name,
			Model:   node.Model,
			Type:    getNodeType(node),
			IsValid: node.IsValid,
			Input:   strings.Join(node.Input, ", "),
			Output:  strings.Join(node.Output, ", "),
			Action:  node.Action,
		}
		tuiNodes = append(tuiNodes, tuiNode)
	}

	// Add parallel group nodes
	for groupName, nodes := range chart.ParallelGroups {
		// Add a parent node for the parallel group
		parentNode := &tui.ChartNode{
			Name:    groupName,
			Model:   "parallel",
			Type:    "parallel",
			IsValid: true,
		}
		tuiNodes = append(tuiNodes, parentNode)

		// Add child nodes
		for _, node := range nodes {
			tuiNode := &tui.ChartNode{
				Name:    node.Name,
				Model:   node.Model,
				Type:    "step",
				IsValid: node.IsValid,
				Input:   strings.Join(node.Input, ", "),
				Output:  strings.Join(node.Output, ", "),
				Action:  node.Action,
			}
			tuiNodes = append(tuiNodes, tuiNode)
		}
	}

	// Add agentic loop nodes
	for loopName, cfg := range chart.AgenticLoopCfgs {
		// Get model from first step if available
		model := "agentic"
		if len(cfg.Steps) > 0 {
			if m, ok := cfg.Steps[0].Config.Model.(string); ok && m != "" {
				model = m
			}
		}
		tuiNode := &tui.ChartNode{
			Name:  loopName,
			Model: model,
			Type:  "loop",
			LoopConfig: &tui.LoopConfig{
				MaxIterations: cfg.MaxIterations,
				ExitCondition: cfg.ExitCondition,
				ContextWindow: cfg.ContextWindow,
			},
			IsValid: true,
		}
		tuiNodes = append(tuiNodes, tuiNode)
	}

	// Calculate stats
	stats := tui.ChartStats{
		TotalSteps:    len(chart.Nodes),
		ParallelSteps: 0,
		LoopCount:     len(chart.AgenticLoopCfgs),
		ValidSteps:    0,
		DeferredSteps: len(chart.DeferredSteps),
		Models:        make(map[string]int),
	}

	for _, nodes := range chart.ParallelGroups {
		stats.ParallelSteps += len(nodes)
	}
	stats.TotalSteps += stats.ParallelSteps

	for _, node := range chart.Nodes {
		if node.IsValid {
			stats.ValidSteps++
		}
		if node.Model != "" && node.Model != modelNA {
			stats.Models[node.Model]++
		}
	}
	for _, nodes := range chart.ParallelGroups {
		for _, node := range nodes {
			if node.IsValid {
				stats.ValidSteps++
			}
			if node.Model != "" && node.Model != modelNA {
				stats.Models[node.Model]++
			}
		}
	}

	workflowName := filepath.Base(filename)
	return tui.RunChart(workflowName, tuiNodes, stats)
}

// getNodeType determines the TUI node type from a ChartNode
func getNodeType(node ChartNode) string {
	switch node.StepType {
	case "agentic-loop":
		return "loop"
	case "generate", "process":
		return "step"
	default:
		if node.IsParallel {
			return "parallel"
		}
		return "step"
	}
}

// renderMermaidChart outputs a Mermaid flowchart representation
func renderMermaidChart(chart *WorkflowChart, filename string) {
	fmt.Println("```mermaid")
	fmt.Println("flowchart TD")

	// Create a safe ID from name (replace special chars)
	safeID := func(name string) string {
		id := strings.ReplaceAll(name, "-", "_")
		id = strings.ReplaceAll(id, " ", "_")
		id = strings.ReplaceAll(id, ":", "_")
		return id
	}

	// Node label with details
	nodeLabel := func(node ChartNode) string {
		label := node.Name
		if node.Model != "" && node.Model != modelNA && node.Model != "[]" {
			label += "<br/><i>" + node.Model + "</i>"
		}
		summary := summarizeAction(node.Action, node.StepType)
		if summary != "" && len(summary) < 40 {
			label += "<br/><small>" + summary + "</small>"
		}
		return label
	}

	// Track node IDs for connections
	nodeIDs := make(map[string]string)
	var prevID string

	// Entry point
	entryText := getEntryPointText(chart)
	if entryText != "" {
		fmt.Printf("    INPUT[[\"%s\"]]\n", entryText)
		prevID = "INPUT"
	}

	// Render nodes in order
	for _, item := range chart.SequentialOrder {
		if strings.HasPrefix(item, "parallel:") {
			groupName := strings.TrimPrefix(item, "parallel:")
			nodes := chart.ParallelGroups[groupName]

			// Create subgraph for parallel group
			groupID := safeID("parallel_" + groupName)
			fmt.Printf("    subgraph %s [\"%s Parallel: %s\"]\n", groupID, parallelIcon, groupName)
			fmt.Println("    direction TB")

			for _, node := range nodes {
				nodeID := safeID(node.Name)
				nodeIDs[node.Name] = nodeID

				// Use different shapes based on step type
				if node.StepType == "agentic-loop" {
					fmt.Printf("        %s{{{\"%s\"}}}\n", nodeID, nodeLabel(node))
				} else {
					fmt.Printf("        %s[\"%s\"]\n", nodeID, nodeLabel(node))
				}
			}
			fmt.Println("    end")

			// Connect previous to all parallel nodes
			if prevID != "" {
				fmt.Printf("    %s --> %s\n", prevID, groupID)
			}
			prevID = groupID

		} else if strings.HasPrefix(item, "agentic-loop:") {
			loopKey := strings.TrimPrefix(item, "agentic-loop:")
			loopCfg, ok := chart.AgenticLoopCfgs[loopKey]
			if !ok {
				continue
			}

			displayName := loopKey
			if loopCfg.Name != "" {
				displayName = loopCfg.Name
			}

			loopID := safeID("loop_" + loopKey)
			nodeIDs[loopKey] = loopID

			// Create subgraph for agentic loop
			maxIter := loopCfg.MaxIterations
			if maxIter == 0 {
				maxIter = 10
			}
			fmt.Printf("    subgraph %s [\"%s %s (max %d iter)\"]\n", loopID, loopIcon, displayName, maxIter)
			fmt.Println("    direction TB")

			var stepIDs []string
			for i, step := range loopCfg.Steps {
				node := stepToChartNode(step, false, "")
				stepID := safeID(fmt.Sprintf("%s_step_%d", loopKey, i))
				stepIDs = append(stepIDs, stepID)
				fmt.Printf("        %s[\"%s\"]\n", stepID, nodeLabel(node))
			}

			// Connect steps within loop
			for i := 0; i < len(stepIDs)-1; i++ {
				fmt.Printf("        %s --> %s\n", stepIDs[i], stepIDs[i+1])
			}

			// Loop back arrow
			if len(stepIDs) > 0 {
				fmt.Printf("        %s -.->|loop| %s\n", stepIDs[len(stepIDs)-1], stepIDs[0])
			}

			fmt.Println("    end")

			// Connect to previous
			if prevID != "" {
				fmt.Printf("    %s --> %s\n", prevID, loopID)
			}
			prevID = loopID

		} else {
			// Regular node
			for _, node := range chart.Nodes {
				if node.Name == item {
					nodeID := safeID(node.Name)
					nodeIDs[node.Name] = nodeID

					// Use different shapes based on step type
					if node.StepType == "agentic-loop" {
						fmt.Printf("    %s{{{\"%s\"}}}\n", nodeID, nodeLabel(node))
					} else if node.StepType == "generate" {
						fmt.Printf("    %s>[\"%s\"]]\n", nodeID, nodeLabel(node))
					} else {
						fmt.Printf("    %s[\"%s\"]\n", nodeID, nodeLabel(node))
					}

					// Connect to previous
					if prevID != "" {
						fmt.Printf("    %s --> %s\n", prevID, nodeID)
					}
					prevID = nodeID
					break
				}
			}
		}
	}

	// Exit point
	exitText := getExitPointText(chart)
	if exitText != "" && prevID != "" {
		fmt.Printf("    OUTPUT[[\"%s\"]]\n", exitText)
		fmt.Printf("    %s --> OUTPUT\n", prevID)
	}

	// Style definitions
	fmt.Println()
	fmt.Println("    %% Styling")
	fmt.Println("    classDef loopStyle fill:#e1f5fe,stroke:#0277bd")
	fmt.Println("    classDef parallelStyle fill:#f3e5f5,stroke:#7b1fa2")
	fmt.Println("    classDef ioStyle fill:#fff3e0,stroke:#ef6c00")

	fmt.Println("```")
}
