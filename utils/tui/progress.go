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

// GetResourceUsage returns current CPU and memory usage
func (p *ProgressReporter) GetResourceUsage() (cpuPercent float64, memoryMB float64) {
	if p.proc == nil {
		return 0, 0
	}

	// Get CPU percent (this is per-process)
	cpu, err := p.proc.CPUPercent()
	if err == nil {
		// Normalize by number of CPUs for more intuitive display
		cpuPercent = cpu / float64(runtime.NumCPU())
	}

	// Get memory info
	memInfo, err := p.proc.MemoryInfo()
	if err == nil && memInfo != nil {
		memoryMB = float64(memInfo.RSS) / 1024 / 1024
	}

	return cpuPercent, memoryMB
}

// Close cleans up all subscribers
func (p *ProgressReporter) Close() {
	for _, ch := range p.subscribers {
		close(ch)
	}
	p.subscribers = nil
}
