//go:build !windows && (!unix || android || ios)

package web

import "fmt"

// canBrowse is false where there is no system browser this package can open
// by itself: a phone's is opened by the host application, a web page's by
// the page.
const canBrowse = false

func systemBrowse(u string) error {
	return fmt.Errorf("%w: no system browser to open %s", ErrUnsupported, u)
}
