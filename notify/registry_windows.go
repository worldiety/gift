//go:build windows

package notify

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// This file is the identity of the application towards Windows: the registry
// entry of its AppUserModelID. The project plan, section 42, records the
// choice of the registry over a Start menu shortcut.
//
//	HKCU\Software\Classes\AppUserModelId\<AppID>
//		DisplayName      the name above every toast
//		IconUri          the icon, if Config.Icon names one
//		CustomActivator  {CLSID} of the class a click is delivered to
//
// It is what the Windows Community Toolkit and the Windows App SDK write for
// an unpackaged application. Values are written only when they differ, so that
// a running application does not touch the registry on every start.

const aumidKey = `Software\Classes\AppUserModelId\`

// register writes the entry for cfg. The class id is the one
// [activatorCLSID] derives from the AppID.
func register(cfg openConfig) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, aumidKey+cfg.appID, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("gift/notify: creating HKCU\\%s%s: %w", aumidKey, cfg.appID, err)
	}
	defer k.Close()

	name := cfg.appName
	if name == "" {
		name = cfg.appID
	}
	values := [][2]string{
		{"DisplayName", name},
		{"CustomActivator", guidString(activatorCLSID(cfg.appID))},
	}
	if cfg.icon != "" {
		icon, err := filepath.Abs(cfg.icon)
		if err != nil {
			return err
		}
		values = append(values, [2]string{"IconUri", icon})
	}
	for _, v := range values {
		if old, _, err := k.GetStringValue(v[0]); err == nil && old == v[1] {
			continue
		}
		if err := k.SetStringValue(v[0], v[1]); err != nil {
			return fmt.Errorf("gift/notify: writing %s of %s: %w", v[0], cfg.appID, err)
		}
	}
	return nil
}

func unregisterPlatform(cfg Config) error {
	if err := checkAUMID(cfg.AppID); err != nil {
		return err
	}
	err := registry.DeleteKey(registry.CURRENT_USER, aumidKey+cfg.AppID)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
