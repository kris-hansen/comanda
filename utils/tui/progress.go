package tui

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// ProgressEvent represents an event during workflow processing
type ProgressEvent struct {
	Type        string // "step_start", "step_end", "loop_iter", "tool_call", "output", "error", "complete"
	Timestamp   time.Time
	StepName    string
	LoopName    string
	Iteration   int
	MaxIter     int
	Message     string
	TokensUsed  int
	TokensAvail int
	Error       error
}

// ProgressReporter sends progress events to subscribers
type ProgressReporter struct {
	subscribers []chan ProgressEvent
	pid         int
	proc        *process.Process
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter() *ProgressReporter {
	pid := os.Getpid()
	proc, _ := process.NewProcess(int32(pid))

	return &ProgressReporter{
		subscribers: make([]chan ProgressEvent, 0),
		pid:         pid,
		proc:        proc,
	}
}

// Subscribe adds a subscriber channel
func (p *ProgressReporter) Subscribe() chan ProgressEvent {
	ch := make(chan ProgressEvent, 100)
	p.subscribers = append(p.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber channel
func (p *ProgressReporter) Unsubscribe(ch chan ProgressEvent) {
	for i, sub := range p.subscribers {
		if sub == ch {
			p.subscribers = append(p.subscribers[:i], p.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// Emit sends an event to all subscribers
func (p *ProgressReporter) Emit(event ProgressEvent) {
	event.Timestamp = time.Now()
	for _, ch := range p.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
}

// StepStart emits a step start event
func (p *ProgressReporter) StepStart(name string) {
	p.Emit(ProgressEvent{
		Type:     "step_start",
		StepName: name,
	})
}

// StepEnd emits a step end event
func (p *ProgressReporter) StepEnd(name string, err error) {
	p.Emit(ProgressEvent{
		Type:     "step_end",
		StepName: name,
		Error:    err,
	})
}

// LoopIteration emits a loop iteration event
func (p *ProgressReporter) LoopIteration(loopName string, iteration, maxIter int) {
	p.Emit(ProgressEvent{
		Type:      "loop_iter",
		LoopName:  loopName,
		Iteration: iteration,
		MaxIter:   maxIter,
	})
}

// ToolCall emits a tool call event
func (p *ProgressReporter) ToolCall(message string) {
	p.Emit(ProgressEvent{
		Type:    "tool_call",
		Message: message,
	})
}

// Output emits an output event
func (p *ProgressReporter) Output(message string) {
	p.Emit(ProgressEvent{
		Type:    "output",
		Message: message,
	})
}

// TokenUpdate emits a token usage update
func (p *ProgressReporter) TokenUpdate(used, available int) {
	p.Emit(ProgressEvent{
		Type:        "tokens",
		TokensUsed:  used,
		TokensAvail: available,
	})
}

// Complete emits a completion event
func (p *ProgressReporter) Complete(err error) {
	p.Emit(ProgressEvent{
		Type:  "complete",
		Error: err,
	})
}

// GetResourceUsage returns current CPU and memory usage for this process and all children
func (p *ProgressReporter) GetResourceUsage() (cpuPercent float64, memoryMB float64) {
	if p.proc == nil {
		return 0, 0
	}

	// Collect all processes to monitor (self + children recursively)
	procs := p.collectProcessTree(p.proc)

	var totalCPU float64
	var totalMemory uint64

	for _, proc := range procs {
		// Get CPU percent
		if cpu, err := proc.CPUPercent(); err == nil {
			totalCPU += cpu
		}

		// Get memory info
		if memInfo, err := proc.MemoryInfo(); err == nil && memInfo != nil {
			totalMemory += memInfo.RSS
		}
	}

	// Normalize CPU by number of CPUs for more intuitive display
	cpuPercent = totalCPU / float64(runtime.NumCPU())
	memoryMB = float64(totalMemory) / 1024 / 1024

	return cpuPercent, memoryMB
}

// collectProcessTree recursively collects a process and all its children
func (p *ProgressReporter) collectProcessTree(proc *process.Process) []*process.Process {
	visited := make(map[int32]bool)
	return p.collectProcessTreeWithVisited(proc, visited)
}

// collectProcessTreeWithVisited recursively collects processes, tracking visited PIDs to prevent cycles
func (p *ProgressReporter) collectProcessTreeWithVisited(proc *process.Process, visited map[int32]bool) []*process.Process {
	// Prevent cycles (shouldn't happen with PIDs, but defensive)
	if visited[proc.Pid] {
		return nil
	}
	visited[proc.Pid] = true

	procs := []*process.Process{proc}

	children, err := proc.Children()
	if err != nil || len(children) == 0 {
		return procs
	}

	for _, child := range children {
		procs = append(procs, p.collectProcessTreeWithVisited(child, visited)...)
	}

	return procs
}

// Close cleans up all subscribers
func (p *ProgressReporter) Close() {
	for _, ch := range p.subscribers {
		close(ch)
	}
	p.subscribers = nil
}
