//go:build (!linux && !darwin) || ios

package system

import (
	"context"
	"math"
	"os"
	"runtime"
	"time"
)

// otherSystem is a platform gift knows nothing of: it names the host and
// refuses every change.
type otherSystem struct{}

func open() System { return otherSystem{} }

func (otherSystem) Platform() Platform {
	return Platform{Kind: OtherKind, Name: runtime.GOOS}
}
func (otherSystem) Can(Capability) bool { return false }
func (otherSystem) Info(context.Context) (Info, error) {
	h, _ := os.Hostname()
	return Info{Hostname: h, OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Temperature: math.NaN()}, nil
}
func (otherSystem) SetHostname(context.Context, string) error           { return ErrReadOnly }
func (otherSystem) Storage(context.Context) (Storage, error)            { return Storage{}, ErrUnsupported }
func (otherSystem) CleanTemporary(context.Context) (int64, error)       { return 0, ErrReadOnly }
func (otherSystem) Interfaces(context.Context) ([]Interface, error)     { return nil, ErrUnsupported }
func (otherSystem) SetWifi(context.Context, bool) error                 { return ErrReadOnly }
func (otherSystem) Scan(context.Context, string) ([]AccessPoint, error) { return nil, ErrReadOnly }
func (otherSystem) Join(context.Context, string, Join) error            { return ErrReadOnly }
func (otherSystem) Forget(context.Context, string) error                { return ErrReadOnly }
func (otherSystem) Configure(context.Context, string, IPConfig) (Change, error) {
	return nil, ErrReadOnly
}
func (otherSystem) CheckUpdates(context.Context) ([]Package, error)      { return nil, ErrReadOnly }
func (otherSystem) InstallUpdates(context.Context, func(Progress)) error { return ErrReadOnly }
func (otherSystem) Release(context.Context) (Release, error)             { return Release{}, ErrReadOnly }
func (otherSystem) UpgradeRelease(context.Context, func(Progress)) error { return ErrReadOnly }
func (otherSystem) Reboot(context.Context) error                         { return ErrReadOnly }
func (otherSystem) PowerOff(context.Context) error                       { return ErrReadOnly }
func (otherSystem) Startup(context.Context) (Startup, error)             { return Startup{Desktop: true}, nil }
func (otherSystem) Install(context.Context, Installation) error          { return ErrReadOnly }
func (otherSystem) SetBoot(context.Context, Boot, bool) error            { return ErrReadOnly }
func (otherSystem) Display(context.Context) (Display, error)             { return Display{}, ErrUnsupported }
func (otherSystem) SetBrightness(context.Context, float64) error         { return ErrReadOnly }
func (otherSystem) SetRotation(context.Context, Rotation) error          { return ErrReadOnly }
func (otherSystem) Clock(context.Context) (Clock, error) {
	return Clock{Zone: time.Local.String()}, nil
}
func (otherSystem) TimeZones(context.Context) ([]string, error) { return nil, ErrReadOnly }
func (otherSystem) SetTimeZone(context.Context, string) error   { return ErrReadOnly }
func (otherSystem) SetNTP(context.Context, bool) error          { return ErrReadOnly }
