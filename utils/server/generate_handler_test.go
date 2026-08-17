package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

type deadlineCapturingResponseWriter struct {
	header   http.Header
	deadline time.Time
}

func (w *deadlineCapturingResponseWriter) Header() http.Header {
	return w.header
}

func (w *deadlineCapturingResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (w *deadlineCapturingResponseWriter) WriteHeader(int) {}

func (w *deadlineCapturingResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestExtendGenerateWriteDeadline(t *testing.T) {
	capturingWriter := &deadlineCapturingResponseWriter{header: make(http.Header)}
	writer := &responseWriter{
		ResponseWriter: &responseWriter{
			ResponseWriter: capturingWriter,
		},
	}
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)

	if err := extendGenerateWriteDeadline(writer, now); err != nil {
		t.Fatalf("extendGenerateWriteDeadline() error = %v", err)
	}

	want := now.Add(generateResponseWriteTimeout)
	if !capturingWriter.deadline.Equal(want) {
		t.Fatalf("write deadline = %s, want %s", capturingWriter.deadline, want)
	}
}

func TestServerGeneratePromptUsesGenericQMDGuidance(t *testing.T) {
	prompt := buildServerGeneratePrompt("guide", "review authentication changes", nil, "")
	for _, want := range []string{
		"QMD CONTEXT RETRIEVAL:",
		"Build every retrieval query solely from concepts named in the user's request.",
		"If no collection is named, omit the -c flag.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing qmd guidance %q", want)
		}
	}
}
