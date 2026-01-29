package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	DeferredSteps   []ChartNode
	EntryInputs     []string
	FinalOutputs    []string
	SequentialOrder []string
	HasStdinEntry   bool
	HasToolInputs   bool
	HasErrors       bool
	Errors          []string
}

var chartCmd = &cobra.Command{
	Use:   "chart <workflow.yaml>",
	Short: "Display an ASCII chart visualization of a workflow",
	Long: `Display a visual representation of a workflow file showing:
  - Relationship between steps (sequential and parallel)
  - Step names, model names, and brief action summaries
  - Validity of each step
  - Input/output chains between steps
  - Workflow statistics`,
	Example: `  # Display workflow chart
  comanda chart workflow.yaml

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

		// Render and print the chart
		renderChart(chart, workflowFile)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(chartCmd)
}

// buildWorkflowChart parses the DSLConfig and builds a WorkflowChart
func buildWorkflowChart(config *processor.DSLConfig) *WorkflowChart {
	chart := &WorkflowChart{
		Nodes:          []ChartNode{},
		ParallelGroups: make(map[string][]ChartNode),
		DeferredSteps:  []ChartNode{},
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
		node.Model = "N/A"
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
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "|")
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "v")
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
			fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "|")
			fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "v")
		}
	}

	// Exit point box
	fmt.Println()
	exitText := getExitPointText(chart)
	if exitText != "" {
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "|")
		fmt.Println(strings.Repeat(" ", (boxWidth-1)/2) + "v")
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

// printBox prints a double-line box with centered text
func printBox(text string, width int) {
	// Truncate if needed
	if len(text) > width-4 {
		text = text[:width-7] + "..."
	}
	padding := width - 2 - len(text)
	leftPad := padding / 2
	rightPad := padding - leftPad

	fmt.Println("+" + strings.Repeat("=", width-2) + "+")
	fmt.Printf("|%s%s%s|\n", strings.Repeat(" ", leftPad), text, strings.Repeat(" ", rightPad))
	fmt.Println("+" + strings.Repeat("=", width-2) + "+")
}

// printSmallBox prints a single-line box
func printSmallBox(text string, width int) {
	if len(text) > width-4 {
		text = text[:width-7] + "..."
	}
	padding := width - 2 - len(text)
	leftPad := padding / 2
	rightPad := padding - leftPad

	fmt.Println("+" + strings.Repeat("-", width-2) + "+")
	fmt.Printf("|%s%s%s|\n", strings.Repeat(" ", leftPad), text, strings.Repeat(" ", rightPad))
	fmt.Println("+" + strings.Repeat("-", width-2) + "+")
}

// printStepBox prints a step box with name, model, and summary
func printStepBox(name, model, summary string, isValid bool, width int) {
	status := "OK"
	if !isValid {
		status = "!!"
	}

	// Build the content lines
	line1 := fmt.Sprintf("[%s] %s", status, name)
	line2 := fmt.Sprintf("Model:  %s", model)
	line3 := fmt.Sprintf("Action: %s", summary)

	// Truncate lines if needed
	maxContent := width - 4
	if len(line1) > maxContent {
		line1 = line1[:maxContent-3] + "..."
	}
	if len(line2) > maxContent {
		line2 = line2[:maxContent-3] + "..."
	}
	if len(line3) > maxContent {
		line3 = line3[:maxContent-3] + "..."
	}

	// Print the box
	fmt.Println("+" + strings.Repeat("-", width-2) + "+")
	fmt.Printf("| %-*s |\n", width-4, line1)
	if model != "" && model != "N/A" && model != "[]" {
		fmt.Printf("| %-*s |\n", width-4, line2)
	}
	if summary != "" {
		fmt.Printf("| %-*s |\n", width-4, line3)
	}
	fmt.Println("+" + strings.Repeat("-", width-2) + "+")
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
		if node.Model != "" && node.Model != "N/A" && node.Model != "[]" {
			modelCounts[node.Model]++
		}
	}
	for _, nodes := range chart.ParallelGroups {
		for _, node := range nodes {
			if node.Model != "" && node.Model != "N/A" && node.Model != "[]" {
				modelCounts[node.Model]++
			}
		}
	}

	fmt.Println("+" + strings.Repeat("=", width-2) + "+")
	fmt.Printf("| %-*s |\n", width-4, "STATISTICS")
	fmt.Println("|" + strings.Repeat("-", width-2) + "|")
	if loopCount > 0 {
		fmt.Printf("| %-*s |\n", width-4, fmt.Sprintf("Loops: %d agentic", loopCount))
	} else {
		fmt.Printf("| %-*s |\n", width-4, fmt.Sprintf("Steps: %d total, %d parallel", totalSteps, parallelSteps))
	}
	if len(chart.DeferredSteps) > 0 {
		fmt.Printf("| %-*s |\n", width-4, fmt.Sprintf("Deferred: %d conditional", len(chart.DeferredSteps)))
	}
	if loopCount == 0 {
		fmt.Printf("| %-*s |\n", width-4, fmt.Sprintf("Valid: %d/%d", validSteps, totalSteps))
	}

	if len(modelCounts) > 0 {
		fmt.Println("|" + strings.Repeat("-", width-2) + "|")
		var models []string
		for model := range modelCounts {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			modelLine := fmt.Sprintf("%s (%d)", model, modelCounts[model])
			fmt.Printf("| %-*s |\n", width-4, modelLine)
		}
	}
	fmt.Println("+" + strings.Repeat("=", width-2) + "+")
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
	printStepBox(node.Name, node.Model, summary, node.IsValid, boxWidth)
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
	status := "OK"
	if !node.IsValid {
		status = "!!"
	}
	summary := summarizeAction(node.Action, node.StepType)

	line1 := fmt.Sprintf("[%s] %s", status, node.Name)
	line2 := fmt.Sprintf("Model:  %s", node.Model)
	line3 := fmt.Sprintf("Action: %s", summary)

	maxContent := width - 6
	if len(line1) > maxContent {
		line1 = line1[:maxContent-3] + "..."
	}
	if len(line2) > maxContent {
		line2 = line2[:maxContent-3] + "..."
	}
	if len(line3) > maxContent {
		line3 = line3[:maxContent-3] + "..."
	}

	fmt.Println("  +" + strings.Repeat("-", width-4) + "+")
	fmt.Printf("  | %-*s |\n", width-6, line1)
	if node.Model != "" && node.Model != "N/A" && node.Model != "[]" {
		fmt.Printf("  | %-*s |\n", width-6, line2)
	}
	if summary != "" {
		fmt.Printf("  | %-*s |\n", width-6, line3)
	}
	fmt.Println("  +" + strings.Repeat("-", width-4) + "+")
}

// renderParallelGroup draws a parallel execution group
func renderParallelGroup(nodes []ChartNode, groupName string, stepNum int, boxWidth int) {
	// Draw a box around the parallel group
	fmt.Println("+" + strings.Repeat("=", boxWidth-2) + "+")
	header := fmt.Sprintf("PARALLEL: %s (%d steps)", groupName, len(nodes))
	if len(header) > boxWidth-4 {
		header = header[:boxWidth-7] + "..."
	}
	fmt.Printf("| %-*s |\n", boxWidth-4, header)
	fmt.Println("+" + strings.Repeat("-", boxWidth-2) + "+")

	// Render each parallel node as a smaller box inside
	for i, node := range nodes {
		renderNodeInline(node, boxWidth)
		if i < len(nodes)-1 {
			// Separator between parallel nodes
			fmt.Println("  " + strings.Repeat("~", boxWidth-6))
		}
	}

	fmt.Println("+" + strings.Repeat("=", boxWidth-2) + "+")
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
