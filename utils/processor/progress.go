package processor

// ProgressType represents different types of progress updates
type ProgressType int

const (
	ProgressSpinner ProgressType = iota
	ProgressStep
	ProgressComplete
	ProgressError
	ProgressOutput // New type for output events
)

// StepInfo contains detailed information about a processing step
type StepInfo struct {
	Name   string
	Model  string
	Action string
}

// ProgressUpdate represents a progress update from the processor
type ProgressUpdate struct {
	Type    ProgressType
	Message string
	Error   error
	Step    *StepInfo // Optional step information
	Stdout  string    // Content from STDOUT when Type is ProgressOutput
}

// ProgressWriter is an interface for handling progress updates
type ProgressWriter interface {
	WriteProgress(update ProgressUpdate) error
}

// channelProgressWriter implements ProgressWriter by sending updates to a channel
type channelProgressWriter struct {
	ch chan<- ProgressUpdate
}

func NewChannelProgressWriter(ch chan<- ProgressUpdate) ProgressWriter {
	return &channelProgressWriter{ch: ch}
}

func (w *channelProgressWriter) WriteProgress(update ProgressUpdate) error {
	w.ch <- update
	return nil
}
