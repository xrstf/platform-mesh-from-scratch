// Package tui contains the (very modest) interactive parts of the installer.
package tui

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
)

// ANSI escape codes; kept minimal and disabled when not writing to a terminal.
const (
	bold   = "\033[1m"
	dim    = "\033[2m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	reset  = "\033[0m"
)

var colorsEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	info, err := os.Stdout.Stat()

	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}()

func colorize(code, s string) string {
	if !colorsEnabled {
		return s
	}

	return code + s + reset
}

// Bold renders a string in bold.
func Bold(s string) string { return colorize(bold, s) }

// Dim renders a string dimmed.
func Dim(s string) string { return colorize(dim, s) }

// Cyan renders a string in cyan.
func Cyan(s string) string { return colorize(cyan, s) }

// Green renders a string in green.
func Green(s string) string { return colorize(green, s) }

// Yellow renders a string in yellow.
func Yellow(s string) string { return colorize(yellow, s) }

// Prompt asks questions on a terminal.
type Prompt struct {
	in  *bufio.Reader
	out io.Writer
}

// NewPrompt creates a prompt reading from stdin and writing to stdout.
func NewPrompt() *Prompt {
	return &Prompt{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stdout,
	}
}

// Print writes a line of text.
func (p *Prompt) Print(format string, args ...any) {
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Ask asks a question until the given validator accepts the answer. An empty answer
// selects the default value (if there is one).
func (p *Prompt) Ask(question, defaultValue string, validate func(string) error) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Fprintf(p.out, "%s %s ", Cyan("?"), Bold(question))
			fmt.Fprintf(p.out, "%s ", Dim("["+defaultValue+"]"))
		} else {
			fmt.Fprintf(p.out, "%s %s ", Cyan("?"), Bold(question))
		}

		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("cannot read answer: %w", err)
		}

		answer := strings.TrimSpace(line)
		if answer == "" {
			answer = defaultValue
		}

		if validate != nil {
			if err := validate(answer); err != nil {
				p.Print("%s %v", Yellow("!"), err)
				continue
			}
		}

		return answer, nil
	}
}

var domainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// ValidateDomain ensures a string looks like a DNS domain.
func ValidateDomain(s string) error {
	if s == "" {
		return fmt.Errorf("please enter a domain, e.g. mesh.example.com")
	}

	if !domainRegex.MatchString(strings.ToLower(s)) {
		return fmt.Errorf("%q does not look like a valid domain name", s)
	}

	return nil
}

// ValidateIP ensures a string is a valid IP address.
func ValidateIP(s string) error {
	if s == "" {
		return fmt.Errorf("please enter an IP address")
	}

	if net.ParseIP(s) == nil {
		return fmt.Errorf("%q is not a valid IP address", s)
	}

	return nil
}

// ValidateNotEmpty ensures a string is not empty.
func ValidateNotEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("please enter a value")
	}

	return nil
}
