package processor

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProgressDisplay manages visual output for workflow execution
type ProgressDisplay struct {
	styler       *Styler
	mu           sync.Mutex
	enabled      bool
	startTime    time.Time
	currentLoop  string
	currentStep  string
	iteration    int
	maxIter      int
	loopCount    int
	totalLoops   int
	lastUpdate   time.Time
	spinnerIndex int
	spinnerChars []string
}

// NewProgressDisplay creates a new progress display
func NewProgressDisplay(enabled bool) *ProgressDisplay {
	return &ProgressDisplay{
		styler:       NewStyler(DefaultStyleConfig()),
		enabled:      enabled,
		spinnerChars: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

// SetEnabled enables or disables the progress display
func (p *ProgressDisplay) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// StartWorkflow displays the workflow header
func (p *ProgressDisplay) StartWorkflow(name string, loopCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	p.startTime = time.Now()
	p.totalLoops = loopCount
	p.loopCount = 0

	fmt.Println()
	fmt.Println(p.styler.Box(fmt.Sprintf("🔀 %s", name), 50))
	fmt.Println()

	if loopCount > 0 {
		loopWord := "loops"
		if loopCount == 1 {
			loopWord = "loop"
		}
		fmt.Printf("  %s %d %s to execute\n", p.styler.Muted(iconBullet), loopCount, loopWord)
		fmt.Println()
	}
}

// StartPreLoopSteps displays the pre-loop steps header
func (p *ProgressDisplay) StartPreLoopSteps(stepCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Printf("  %s Running %s pre-loop steps\n",
		p.styler.RunningIcon(),
		p.styler.Bold(fmt.Sprintf("%d", stepCount)))
	fmt.Println()
}

// CompletePreLoopSteps marks pre-loop steps as complete
func (p *ProgressDisplay) CompletePreLoopSteps(duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Printf("  %s Pre-loop steps completed %s\n",
		p.styler.SuccessIcon(),
		p.styler.Duration(formatDuration(duration)))
	fmt.Println()
}

// StartLoop displays the loop start header
func (p *ProgressDisplay) StartLoop(name string, loopIndex, totalLoops, maxIterations int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	p.currentLoop = name
	p.loopCount = loopIndex
	p.maxIter = maxIterations
	p.iteration = 0

	progress := ""
	if totalLoops > 1 {
		progress = fmt.Sprintf(" [%d/%d]", loopIndex, totalLoops)
	}

	fmt.Printf("  %s %s%s\n",
		p.styler.LoopIcon(),
		p.styler.LoopName(name),
		p.styler.Muted(progress))
}

// UpdateIteration updates the current iteration display
func (p *ProgressDisplay) UpdateIteration(iteration, maxIterations int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	p.iteration = iteration
	p.maxIter = maxIterations

	// Clear previous line and write new status
	iterStr := p.styler.Iteration(iteration, maxIterations)
	elapsed := time.Since(p.startTime)

	fmt.Printf("     %s Iteration %s %s\n",
		p.styler.StepIcon(),
		iterStr,
		p.styler.Duration(formatDuration(elapsed)))
}

// StartStep displays a step starting within a loop
func (p *ProgressDisplay) StartStep(name string, model string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	p.currentStep = name

	modelStr := ""
	if model != "" && model != "<nil>" {
		modelStr = p.styler.Muted(fmt.Sprintf(" [%s]", model))
	}

	fmt.Printf("       %s %s%s\n",
		p.styler.RunningIcon(),
		p.styler.StepName(name),
		modelStr)
}

// CompleteStep marks a step as complete
func (p *ProgressDisplay) CompleteStep(name string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	// Move cursor up and overwrite the running line
	fmt.Printf("\033[1A\033[2K") // Move up, clear line
	fmt.Printf("       %s %s %s\n",
		p.styler.SuccessIcon(),
		p.styler.StepName(name),
		p.styler.Duration(formatDuration(duration)))
}

// FailStep marks a step as failed
func (p *ProgressDisplay) FailStep(name string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Printf("\033[1A\033[2K") // Move up, clear line
	fmt.Printf("       %s %s\n",
		p.styler.ErrorIcon(),
		p.styler.Error(name))
	fmt.Printf("         %s\n", p.styler.Muted(err.Error()))
}

// CompleteLoop marks a loop as complete
func (p *ProgressDisplay) CompleteLoop(name string, iterations int, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Printf("     %s Completed after %d iterations %s\n",
		p.styler.SuccessIcon(),
		iterations,
		p.styler.Duration(formatDuration(duration)))
	fmt.Println()
}

// FailLoop marks a loop as failed
func (p *ProgressDisplay) FailLoop(name string, iteration int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Printf("     %s Failed at iteration %d\n",
		p.styler.ErrorIcon(),
		iteration)
	fmt.Printf("       %s\n", p.styler.Muted(err.Error()))
	fmt.Println()
}

// CompleteWorkflow displays the workflow completion summary
func (p *ProgressDisplay) CompleteWorkflow(loopResults map[string]*LoopOutput) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	totalDuration := time.Since(p.startTime)

	fmt.Println(p.styler.Divider(50))
	fmt.Println()
	fmt.Printf("  %s Workflow completed %s\n",
		p.styler.SuccessIcon(),
		p.styler.Duration(formatDuration(totalDuration)))
	fmt.Println()

	// Summary table
	if len(loopResults) > 0 {
		fmt.Printf("  %s\n", p.styler.Bold("Summary:"))
		for name, output := range loopResults {
			statusIcon := p.styler.SuccessIcon()
			if output.Status != "completed" {
				statusIcon = p.styler.ErrorIcon()
			}

			duration := output.EndTime.Sub(output.StartTime)
			fmt.Printf("    %s %s %s\n",
				statusIcon,
				p.styler.LoopName(name),
				p.styler.Duration(formatDuration(duration)))
		}
		fmt.Println()
	}
}

// FailWorkflow displays the workflow failure
func (p *ProgressDisplay) FailWorkflow(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	fmt.Println()
	fmt.Println(p.styler.Divider(50))
	fmt.Printf("  %s %s\n",
		p.styler.ErrorIcon(),
		p.styler.Error("Workflow failed"))
	fmt.Printf("    %s\n", p.styler.Muted(err.Error()))
	fmt.Println()
}

// ShowDependencyGraph displays the loop execution order
func (p *ProgressDisplay) ShowDependencyGraph(order []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled || len(order) == 0 {
		return
	}

	fmt.Printf("  %s Execution order:\n", p.styler.Muted("│"))
	for i, name := range order {
		isLast := i == len(order)-1
		branch := p.styler.TreeBranch(isLast)
		fmt.Printf("  %s%s\n", branch, p.styler.LoopName(name))
	}
	fmt.Println()
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// LoopProgress shows a compact progress line for a loop
func (p *ProgressDisplay) LoopProgress(name string, iteration, maxIter int, step string, elapsed time.Duration) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	parts := []string{
		p.styler.LoopIcon(),
		p.styler.LoopName(name),
		p.styler.Muted("│"),
		p.styler.Iteration(iteration, maxIter),
	}

	if step != "" {
		parts = append(parts, p.styler.Muted("│"), p.styler.StepName(step))
	}

	parts = append(parts, p.styler.Muted("│"), p.styler.Duration(formatDuration(elapsed)))

	return strings.Join(parts, " ")
}
