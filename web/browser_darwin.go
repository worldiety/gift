//go:build darwin && !ios

package web

import (
	"fmt"
	"os/exec"
)

const canBrowse = true

// systemBrowse opens u with open(1), which hands it to LaunchServices and so to
// the default browser.
func systemBrowse(u string) error { return run(exec.Command("open", u)) }

func openerError(name string, err error) error {
	if err != nil {
		return fmt.Errorf("gift/web: %s: %w", name, err)
	}
	return nil
}
