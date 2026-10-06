package processor

import (
	"bytes"
	"strings"
	"testing"
)

func TestSpinnerShorterFrameClearsPreviousText(t *testing.T) {
	for _, tc := range []struct {
		name string
		tty  bool
	}{
		{name: "terminal", tty: true},
		{name: "redirected output", tty: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			lastWidth := 0
			writeSpinnerLine(&output, "Building a very long graph phase", tc.tty, &lastWidth, false)
			writeSpinnerLine(&output, "Done", tc.tty, &lastWidth, true)
			frames := strings.Split(output.String(), "\r")
			last := frames[len(frames)-1]
			if tc.tty {
				if last != "\033[2KDone\n" {
					t.Fatalf("shorter terminal frame did not clear line: %q", last)
				}
			} else if !strings.HasPrefix(last, "Done") || !strings.HasSuffix(last, "\n") || strings.TrimSpace(last) != "Done" {
				t.Fatalf("shorter redirected frame left old text: %q", last)
			}
		})
	}
}
