//go:build unix && !android && !ios

package web

import (
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// openerWait is how long an opener is waited for: long enough for one that
// fails at once to say so, and no longer, because xdg-open runs some browsers
// in the foreground and returns only when the browser is closed.
const openerWait = 3 * time.Second

// run starts an opener and waits for it a little.
func run(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return fmt.Errorf("gift/web: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return openerError(cmd.Path, err)
	case <-time.After(openerWait):
		return nil
	}
}
