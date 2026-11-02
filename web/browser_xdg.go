//go:build unix && !darwin && !android

package web

import (
	"errors"
	"fmt"
	"os/exec"
)

const canBrowse = true

// systemBrowse opens u with xdg-open, which asks the desktop for the default
// browser; in a Flatpak, the xdg-open of the runtime asks the OpenURI portal.
func systemBrowse(u string) error { return run(exec.Command("xdg-open", u)) }

// openerError reads the exit status of xdg-open: 3 is "a required tool could
// not be found", no browser, which is what a kiosk without a desktop says.
func openerError(name string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 3 {
		return fmt.Errorf("%w: %s found no browser", ErrUnsupported, name)
	}
	if err != nil {
		return fmt.Errorf("gift/web: %s: %w", name, err)
	}
	return nil
}
