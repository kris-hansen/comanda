package processor

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type Spinner struct {
	chars    []string
	index    int
	message  string
	detail   string
	stop     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopped  bool
	disabled bool // Used for testing environments
	progress ProgressWriter
}

func NewSpinner() *Spinner {
	return &Spinner{
		chars: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stop:  make(chan struct{}),
	}
}

func (s *Spinner) SetProgressWriter(w ProgressWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = w
}

// Disable prevents the spinner from showing any output
func (s *Spinner) Disable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabled = true
}

func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.disabled {
		s.mu.Unlock()
		return
	}
	if s.stopped {
		s.stop = make(chan struct{})
		s.stopped = false
	}
	s.message = message
	s.detail = ""
	s.mu.Unlock()

	// Send initial progress update
	if s.progress != nil {
		s.progress.WriteProgress(ProgressUpdate{
			Type:    ProgressStep,
			Message: message,
		})
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// Hide cursor during spinner animation (only if stdout is a terminal)
		isTTY := term.IsTerminal(int(os.Stdout.Fd()))
		lastWidth := 0
		if isTTY {
			fmt.Print("\033[?25l")
		}
		for {
			select {
			case <-s.stop:
				s.mu.Lock()
				msg := fmt.Sprintf("%s... Done!", s.message)
				disabled := s.disabled
				progress := s.progress
				s.mu.Unlock()

				if !disabled {
					writeSpinnerLine(os.Stdout, msg, isTTY, &lastWidth, true)
				}
				// Show cursor again (only if stdout is a terminal)
				if isTTY {
					fmt.Print("\033[?25h")
				}
				// Send completion update
				if progress != nil {
					progress.WriteProgress(ProgressUpdate{
						Type:    ProgressStep,
						Message: msg,
					})
				}
				return
			default:
				s.mu.Lock()
				if !s.disabled {
					spinMsg := fmt.Sprintf("%s... %s", s.message, s.chars[s.index])
					if s.detail != "" {
						spinMsg += fmt.Sprintf("  ·  %s", s.detail)
					}
					writeSpinnerLine(os.Stdout, spinMsg, isTTY, &lastWidth, false)
					// Don't send spinner updates through progress writer
					s.index = (s.index + 1) % len(s.chars)
				}
				s.mu.Unlock()
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// writeSpinnerLine clears the previous frame before drawing the next one.
// Non-terminal output is padded instead of receiving ANSI escape sequences.
func writeSpinnerLine(w io.Writer, line string, isTTY bool, lastWidth *int, finish bool) {
	if isTTY {
		fmt.Fprintf(w, "\r\033[2K%s", line)
	} else {
		width := utf8.RuneCountInString(line)
		fmt.Fprintf(w, "\r%s%s", line, strings.Repeat(" ", max(0, *lastWidth-width)))
		*lastWidth = width
	}
	if finish {
		fmt.Fprint(w, "\n")
	}
}

// SetProgress updates the live spinner label and optional detail. Commands
// with long deterministic work can show both a phase and the file or object
// currently being processed without starting a second UI.
func (s *Spinner) SetProgress(message, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
	s.detail = detail
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.stopped {
		close(s.stop)
		s.stopped = true
	}
	s.mu.Unlock()
	s.wg.Wait()
}
