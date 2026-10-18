package main

import (
	"fmt"
	"time"
)

const banner = `
 ____  _____ __  __ _____ _____    _    __  __ ____
/ ___|| ____|  \/  |_   _| ____|  / \  |  \/  / ___|
\___ \|  _| | |\/| | | | |  _|   / _ \ | |\/| \___ \
 ___) | |___| |  | | | | | |___ / ___ \| |  | |___) |
|____/|_____|_|  |_| |_| |_____/_/   \_\_|  |_|____/

    always-on program manager for a configurable portfolio
`

func printBanner() {
	fmt.Print(banner)
	fmt.Printf("  version %s\n\n", version)
}

// Spinner provides a simple animated spinner for long-running operations.
type Spinner struct {
	frames  []string
	current int
	message string
	done    chan struct{}
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message: message,
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation in a goroutine.
func (s *Spinner) Start() {
	go func() {
		for {
			select {
			case <-s.done:
				return
			default:
				fmt.Printf("\r%s %s", s.frames[s.current], s.message)
				s.current = (s.current + 1) % len(s.frames)
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
}

// Stop stops the spinner and prints a success checkmark.
func (s *Spinner) Stop() {
	close(s.done)
	// Small delay to ensure goroutine processes the close
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("\r✓ %s\n", s.message)
}

// StopWithError stops the spinner and prints a failure mark.
func (s *Spinner) StopWithError(err error) {
	close(s.done)
	time.Sleep(10 * time.Millisecond)
	fmt.Printf("\r✗ %s: %v\n", s.message, err)
}
