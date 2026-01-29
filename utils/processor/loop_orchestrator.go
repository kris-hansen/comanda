package processor

import (
	"fmt"
	"time"
)

// LoopOrchestrator manages execution of multiple interdependent loops
type LoopOrchestrator struct {
	processor      *Processor
	loops          map[string]*AgenticLoopConfig
	loopOutputs    map[string]*LoopOutput
	executionGraph *DependencyGraph
	workflowFile   string
}

// DependencyGraph represents a directed acyclic graph of loop dependencies
type DependencyGraph struct {
	nodes        map[string]*GraphNode
	edges        map[string][]string // loopName -> list of dependencies
	reverseEdges map[string][]string // loopName -> list of dependents
}

// GraphNode represents a node in the dependency graph
type GraphNode struct {
	name         string
	config       *AgenticLoopConfig
	dependencies []string
	dependents   []string
}

// NewLoopOrchestrator creates a new loop orchestrator
func NewLoopOrchestrator(processor *Processor, loops map[string]*AgenticLoopConfig, workflowFile string) *LoopOrchestrator {
	return &LoopOrchestrator{
		processor:    processor,
		loops:        loops,
		loopOutputs:  make(map[string]*LoopOutput),
		workflowFile: workflowFile,
	}
}

// Execute runs all loops in dependency order
func (o *LoopOrchestrator) Execute() error {
	o.processor.debugf("Starting loop orchestration with %d loops", len(o.loops))

	// Build dependency graph
	graph, err := o.buildDependencyGraph()
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}
	o.executionGraph = graph

	// Perform topological sort
	executionOrder, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("failed to determine execution order: %w", err)
	}

	o.processor.debugf("Execution order: %v", executionOrder)

	// Execute loops in order
	for _, loopName := range executionOrder {
		config := o.loops[loopName]
		if config == nil {
			return fmt.Errorf("loop '%s' not found in configuration", loopName)
		}

		o.processor.debugf("Executing loop: %s", loopName)

		// Prepare input for this loop
		input, err := o.prepareLoopInput(config)
		if err != nil {
			return fmt.Errorf("failed to prepare input for loop '%s': %w", loopName, err)
		}

		// Execute the loop
		startTime := time.Now()
		result, err := o.processor.processAgenticLoopWithFile(loopName, config, input, o.workflowFile)
		endTime := time.Now()

		// Record output
		output := &LoopOutput{
			LoopName:  loopName,
			StartTime: startTime,
			EndTime:   endTime,
			Result:    result,
			Variables: make(map[string]string),
		}

		if err != nil {
			output.Status = "failed"
			o.loopOutputs[loopName] = output
			return fmt.Errorf("loop '%s' failed: %w", loopName, err)
		}

		output.Status = "completed"

		// Export variables if output_state is specified
		if config.OutputState != "" {
			o.processor.debugf("Exporting output to variable: %s", config.OutputState)
			o.processor.variables[config.OutputState] = result
			output.Variables[config.OutputState] = result
		}

		o.loopOutputs[loopName] = output
		o.processor.debugf("Loop '%s' completed successfully", loopName)
	}

	o.processor.debugf("All loops completed successfully")
	return nil
}

// buildDependencyGraph constructs a dependency graph from loop configurations
func (o *LoopOrchestrator) buildDependencyGraph() (*DependencyGraph, error) {
	graph := &DependencyGraph{
		nodes:        make(map[string]*GraphNode),
		edges:        make(map[string][]string),
		reverseEdges: make(map[string][]string),
	}

	// Create nodes
	for name, config := range o.loops {
		node := &GraphNode{
			name:         name,
			config:       config,
			dependencies: config.DependsOn,
			dependents:   []string{},
		}
		graph.nodes[name] = node
		graph.edges[name] = config.DependsOn
	}

	// Build reverse edges (dependents)
	for name, deps := range graph.edges {
		for _, dep := range deps {
			if _, exists := graph.nodes[dep]; !exists {
				return nil, fmt.Errorf("loop '%s' depends on non-existent loop '%s'", name, dep)
			}
			graph.reverseEdges[dep] = append(graph.reverseEdges[dep], name)
			graph.nodes[dep].dependents = append(graph.nodes[dep].dependents, name)
		}
	}

	return graph, nil
}

// prepareLoopInput prepares the input for a loop based on its dependencies
func (o *LoopOrchestrator) prepareLoopInput(config *AgenticLoopConfig) (string, error) {
	// If input_state is specified, use that variable
	if config.InputState != "" {
		value, exists := o.processor.variables[config.InputState]
		if !exists {
			return "", fmt.Errorf("input variable '%s' not found", config.InputState)
		}
		o.processor.debugf("Using input from variable: %s", config.InputState)
		return value, nil
	}

	// If depends_on is specified, use the result of the first dependency
	if len(config.DependsOn) > 0 {
		firstDep := config.DependsOn[0]
		output, exists := o.loopOutputs[firstDep]
		if !exists {
			return "", fmt.Errorf("dependency loop '%s' has not completed", firstDep)
		}
		o.processor.debugf("Using input from dependency loop: %s", firstDep)
		return output.Result, nil
	}

	// No dependencies, start with empty input
	return "NA", nil
}

// TopologicalSort performs topological sort using Kahn's algorithm
// Returns execution order or error if cycle detected
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	// Calculate in-degrees
	inDegree := make(map[string]int)
	for name := range g.nodes {
		inDegree[name] = len(g.edges[name])
	}

	// Queue for nodes with no dependencies
	queue := []string{}
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Process queue
	result := []string{}
	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Process dependents
		for _, dependent := range g.reverseEdges[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for cycles
	if len(result) != len(g.nodes) {
		// Find cycle
		cycle := g.findCycle()
		if len(cycle) > 0 {
			return nil, fmt.Errorf("dependency cycle detected: %s", formatCycle(cycle))
		}
		return nil, fmt.Errorf("dependency cycle detected (unable to determine specific cycle)")
	}

	return result, nil
}

// findCycle attempts to find a cycle in the graph using DFS
func (g *DependencyGraph) findCycle() []string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	parent := make(map[string]string)

	var dfs func(string) []string
	dfs = func(node string) []string {
		visited[node] = true
		recStack[node] = true

		for _, dep := range g.edges[node] {
			if !visited[dep] {
				parent[dep] = node
				if cycle := dfs(dep); cycle != nil {
					return cycle
				}
			} else if recStack[dep] {
				// Found cycle, reconstruct it
				cycle := []string{dep}
				current := node
				for current != dep {
					cycle = append([]string{current}, cycle...)
					current = parent[current]
				}
				cycle = append(cycle, dep) // Close the cycle
				return cycle
			}
		}

		recStack[node] = false
		return nil
	}

	for node := range g.nodes {
		if !visited[node] {
			if cycle := dfs(node); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// formatCycle formats a cycle for error messages
func formatCycle(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	result := cycle[0]
	for i := 1; i < len(cycle); i++ {
		result += " -> " + cycle[i]
	}
	return result
}

// GetLoopOutput returns the output of a completed loop
func (o *LoopOrchestrator) GetLoopOutput(loopName string) (*LoopOutput, error) {
	output, exists := o.loopOutputs[loopName]
	if !exists {
		return nil, fmt.Errorf("loop '%s' has not completed or does not exist", loopName)
	}
	return output, nil
}

// GetAllOutputs returns outputs of all completed loops
func (o *LoopOrchestrator) GetAllOutputs() map[string]*LoopOutput {
	return o.loopOutputs
}
