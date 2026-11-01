//go:build linux && !android

package notify

import (
	"fmt"
	"os"
)

// openPlatform connects to the session bus and to the notification server on
// it. Both failures are [ErrUnsupported]: a machine without a session bus or
// without a server is a machine without notifications, which is a normal
// state for a kiosk and not a defect.
func openPlatform(cfg openConfig) (platform, error) {
	addr, err := sessionBusAddress(os.Getenv, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	c, err := dialBus(addr, callTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot connect to the session bus: %v", ErrUnsupported, err)
	}
	f, err := newFreedesktop(c, cfg)
	if err != nil {
		c.close()
		return nil, err
	}
	c.setSignalHandler(f.signal)
	return f, nil
}

func unregisterPlatform(Config) error { return nil }
