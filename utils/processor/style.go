package processor

import (
	"fmt"
	"os"
	"strings"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorItalic = "\033[3m"

	// Foreground colors
	colorBlack   = "\033[30m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"

	// Bright foreground colors
	colorBrightBlack   = "\033[90m"
	colorBrightRed     = "\033[91m"
	colorBrightGreen   = "\033[92m"
	colorBrightYellow  = "\033[93m"
	colorBrightBlue    = "\033[94m"
	colorBrightMagenta = "\033[95m"
	colorBrightCyan    = "\033[96m"
	colorBrightWhite   = "\033[97m"
)

// Unicode box drawing characters
const (
	boxHorizontal      = "─"
	boxVertical        = "│"
	boxTopLeft         = "╭"
	boxTopRight        = "╮"
	boxBottomLeft      = "╰"
	boxBottomRight     = "╯"
	boxVerticalRight   = "├"
	boxVerticalLeft    = "┤"
	boxHorizontalDown  = "┬"
	boxHorizontalUp    = "┴"
	boxCross           = "┼"
	boxDoubleHorizontal = "═"
	boxDoubleVertical   = "║"
)

// Status icons
const (
	iconSuccess    = "✓"
	iconError      = "✗"
	iconWarning    = "⚠"
	iconRunning    = "⏳"
	iconPending    = "○"
	iconLoop       = "🔄"
	iconStep       = "→"
	iconArrowDown  = "↓"
	iconArrowRight = "→"
	iconBullet     = "•"
	iconCheck      = "✓"
	iconCross      = "✗"
	iconStar       = "★"
	iconDot        = "·"
)

// StyleConfig controls output styling behavior
type StyleConfig struct {
	UseColors    bool
	UseUnicode   bool
	CompactMode  bool
}

// DefaultStyleConfig returns the default style configuration
func DefaultStyleConfig() *StyleConfig {
	// Check if we're in a terminal that supports colors
	useColors := true
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColors = false
	}
	
	return &StyleConfig{
		UseColors:   useColors,
		UseUnicode:  true,
		CompactMode: false,
	}
}

// Styler provides methods for styled terminal output
type Styler struct {
	config *StyleConfig
}

// NewStyler creates a new Styler with the given configuration
func NewStyler(config *StyleConfig) *Styler {
	if config == nil {
		config = DefaultStyleConfig()
	}
	return &Styler{config: config}
}

// color wraps text with ANSI color codes if colors are enabled
func (s *Styler) color(text string, codes ...string) string {
	if !s.config.UseColors || len(codes) == 0 {
		return text
	}
	return strings.Join(codes, "") + text + colorReset
}

// Bold returns bold text
func (s *Styler) Bold(text string) string {
	return s.color(text, colorBold)
}

// Dim returns dimmed text
func (s *Styler) Dim(text string) string {
	return s.color(text, colorDim)
}

// Success returns green success text
func (s *Styler) Success(text string) string {
	return s.color(text, colorGreen)
}

// Error returns red error text
func (s *Styler) Error(text string) string {
	return s.color(text, colorRed)
}

// Warning returns yellow warning text
func (s *Styler) Warning(text string) string {
	return s.color(text, colorYellow)
}

// Info returns cyan info text
func (s *Styler) Info(text string) string {
	return s.color(text, colorCyan)
}

// Highlight returns magenta highlighted text
func (s *Styler) Highlight(text string) string {
	return s.color(text, colorMagenta)
}

// Muted returns dim gray text
func (s *Styler) Muted(text string) string {
	return s.color(text, colorBrightBlack)
}

// Model returns styled model name (cyan + bold)
func (s *Styler) Model(name string) string {
	return s.color(name, colorCyan, colorBold)
}

// LoopName returns styled loop name (magenta + bold)
func (s *Styler) LoopName(name string) string {
	return s.color(name, colorMagenta, colorBold)
}

// StepName returns styled step name (blue)
func (s *Styler) StepName(name string) string {
	return s.color(name, colorBlue)
}

// Iteration returns styled iteration counter
func (s *Styler) Iteration(current, max int) string {
	return s.color(fmt.Sprintf("%d/%d", current, max), colorYellow)
}

// Duration returns styled duration
func (s *Styler) Duration(d string) string {
	return s.color(d, colorBrightBlack)
}

// SuccessIcon returns a green checkmark
func (s *Styler) SuccessIcon() string {
	if !s.config.UseUnicode {
		return "[OK]"
	}
	return s.Success(iconSuccess)
}

// ErrorIcon returns a red X
func (s *Styler) ErrorIcon() string {
	if !s.config.UseUnicode {
		return "[FAIL]"
	}
	return s.Error(iconCross)
}

// RunningIcon returns a running indicator
func (s *Styler) RunningIcon() string {
	if !s.config.UseUnicode {
		return "[...]"
	}
	return s.Warning(iconRunning)
}

// LoopIcon returns a loop indicator
func (s *Styler) LoopIcon() string {
	if !s.config.UseUnicode {
		return "[LOOP]"
	}
	return iconLoop
}

// StepIcon returns a step indicator
func (s *Styler) StepIcon() string {
	if !s.config.UseUnicode {
		return "->"
	}
	return s.Info(iconStep)
}

// Box draws a box around content
func (s *Styler) Box(title string, width int) string {
	if !s.config.UseUnicode {
		return s.asciiBox(title, width)
	}
	
	if width < len(title)+4 {
		width = len(title) + 4
	}
	
	padding := width - len(title) - 2
	leftPad := padding / 2
	rightPad := padding - leftPad
	
	top := boxTopLeft + strings.Repeat(boxHorizontal, width) + boxTopRight
	middle := boxVertical + strings.Repeat(" ", leftPad) + s.Bold(title) + strings.Repeat(" ", rightPad) + boxVertical
	bottom := boxBottomLeft + strings.Repeat(boxHorizontal, width) + boxBottomRight
	
	return top + "\n" + middle + "\n" + bottom
}

func (s *Styler) asciiBox(title string, width int) string {
	if width < len(title)+4 {
		width = len(title) + 4
	}
	
	border := "+" + strings.Repeat("-", width) + "+"
	padding := width - len(title)
	leftPad := padding / 2
	rightPad := padding - leftPad
	middle := "|" + strings.Repeat(" ", leftPad) + title + strings.Repeat(" ", rightPad) + "|"
	
	return border + "\n" + middle + "\n" + border
}

// Divider returns a horizontal line
func (s *Styler) Divider(width int) string {
	if !s.config.UseUnicode {
		return strings.Repeat("-", width)
	}
	return s.Muted(strings.Repeat(boxHorizontal, width))
}

// TreeBranch returns tree-drawing characters for hierarchical output
func (s *Styler) TreeBranch(isLast bool) string {
	if !s.config.UseUnicode {
		if isLast {
			return "`-- "
		}
		return "|-- "
	}
	if isLast {
		return boxBottomLeft + boxHorizontal + boxHorizontal + " "
	}
	return boxVerticalRight + boxHorizontal + boxHorizontal + " "
}

// TreePipe returns the vertical continuation line for trees
func (s *Styler) TreePipe() string {
	if !s.config.UseUnicode {
		return "|   "
	}
	return boxVertical + "   "
}

// ProgressBar returns a progress bar
func (s *Styler) ProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	
	percent := float64(current) / float64(total)
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	percentStr := fmt.Sprintf("%3d%%", int(percent*100))
	
	return s.Info(bar) + " " + s.Muted(percentStr)
}
