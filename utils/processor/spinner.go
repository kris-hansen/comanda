package processor

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Spinner struct {
	chars    []string
	index    int
	message  string
	stop     chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopped  bool
	disabled bool // Used for testing environments
	progress ProgressWriter
}

func NewSpinner() *Spinner {
	return &Spinner{
		chars: []string{"|", "/", "-", "\\"},
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
		for {
			select {
			case <-s.stop:
				s.mu.Lock()
				msg := fmt.Sprintf("%s... Done!", s.message)
				if !s.disabled {
					fmt.Printf("\r%s     \n", msg)
				}
				// Send completion update
				if s.progress != nil {
					s.progress.WriteProgress(ProgressUpdate{
						Type:    ProgressStep,
						Message: msg,
					})
				}
				s.mu.Unlock()
				return
			default:
				s.mu.Lock()
				if !s.disabled {
					spinMsg := fmt.Sprintf("%s... %s", s.message, s.chars[s.index])
					fmt.Printf("\r%s", spinMsg)
					// Don't send spinner updates through progress writer
					s.index = (s.index + 1) % len(s.chars)
				}
				s.mu.Unlock()
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
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
