package runner

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Printer writes the transcript of a run. Lines of one step are indented under its header. It is safe for use from
// several goroutines.
type Printer struct {
	mu       sync.Mutex
	out      io.Writer
	showTime bool
}

func newPrinter(out io.Writer, showTime bool) *Printer { return &Printer{out: out, showTime: showTime} }

func (p *Printer) line(indent int, format string, a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "%s%s\n", strings.Repeat("  ", indent), fmt.Sprintf(format, a...))
}

func paint(result, text string) string {
	switch result {
	case "success":
		return color.GreenString(text)
	case "failure", "cancelled":
		return color.RedString(text)
	case "skipped":
		return color.YellowString(text)
	}
	return text
}

func (p *Printer) JobStart(name string) { p.line(0, "[job] %s", name) }
func (p *Printer) JobSkip(name, why string) {
	p.line(0, "[job] %s: %s (%s)", name, paint("skipped", "skipped"), why)
}
func (p *Printer) JobEnd(name, result string, d time.Duration) {
	p.line(0, "[job] %s: %s%s", name, paint(result, result), p.dur(d))
}

func (p *Printer) StepStart(n label, name string)  { p.line(n.indent(), "[%s] %s", n, name) }
func (p *Printer) StepOutput(n label, line string) { p.line(n.indent()+1, "%s", line) }
func (p *Printer) StepEnd(n label, result, note string, d time.Duration) {
	if note != "" {
		note = " (" + note + ")"
	}
	p.line(n.indent(), "[%s] %s%s%s", n, paint(result, result), note, p.dur(d))
}
func (p *Printer) StepSkip(n label, name, why string) {
	p.line(n.indent(), "[%s] %s: %s (%s)", n, name, paint("skipped", "skipped"), why)
}
func (p *Printer) Note(indent int, format string, a ...any) { p.line(indent, format, a...) }

func (p *Printer) dur(d time.Duration) string {
	if !p.showTime {
		return ""
	}
	return fmt.Sprintf(" [%.1fs]", d.Seconds())
}

// label names a step in the transcript: "step 3", "step 3.1" for a step inside the composite action of step 3, "post 3"
// for the post step of the action of step 3.
type label string

func (l label) indent() int { return 1 + strings.Count(string(l), ".") }
