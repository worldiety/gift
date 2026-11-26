//go:build darwin && !ios

package system

import (
	"bufio"
	"context"
	"math"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// darwinSystem is macOS, read only: gift is no system tool on a Mac, and a
// settings app there shows what the Mac is and leaves every change to the
// System Settings of macOS.
type darwinSystem struct{ plat Platform }

func open() System {
	v, _ := unix.Sysctl("kern.osproductversion")
	return &darwinSystem{plat: Platform{Kind: MacOS, Name: "macOS", Version: v, Root: false}}
}

func (s *darwinSystem) Platform() Platform  { return s.plat }
func (s *darwinSystem) Can(Capability) bool { return false }

func (s *darwinSystem) Info(ctx context.Context) (Info, error) {
	in := Info{Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Temperature: math.NaN()}
	in.Hostname, _ = os.Hostname()
	build, _ := unix.Sysctl("kern.osversion")
	in.OS = strings.TrimSpace("macOS " + s.plat.Version + " (" + build + ")")
	var u unix.Utsname
	if unix.Uname(&u) == nil {
		in.Kernel = "Darwin " + unix.ByteSliceToString(u.Release[:])
	}
	in.Model, _ = unix.Sysctl("hw.model")
	in.CPU, _ = unix.Sysctl("machdep.cpu.brand_string")
	if m, err := unix.SysctlUint64("hw.memsize"); err == nil {
		in.Memory = m
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		in.Uptime = time.Since(time.Unix(tv.Sec, int64(tv.Usec)*1000)).Truncate(time.Second)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if _, v, ok := strings.Cut(line, `"IOPlatformSerialNumber" = `); ok {
				in.Serial = strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
	}
	return in, nil
}

func (s *darwinSystem) SetHostname(context.Context, string) error { return ErrReadOnly }

func (s *darwinSystem) Storage(context.Context) (Storage, error) {
	var st Storage
	for _, p := range []string{"/", "/System/Volumes/Data"} {
		var fs unix.Statfs_t
		if unix.Statfs(p, &fs) != nil {
			continue
		}
		st.Volumes = append(st.Volumes, Volume{Path: p, Device: unix.ByteSliceToString(fs.Mntfromname[:]),
			Type: unix.ByteSliceToString(fs.Fstypename[:]), Total: fs.Blocks * uint64(fs.Bsize), Free: fs.Bavail * uint64(fs.Bsize)})
	}
	if len(st.Volumes) > 0 {
		v := st.Volumes[len(st.Volumes)-1]
		st.Usage = []Usage{{Name: "System", Bytes: int64(v.Total - v.Free)}}
	}
	return st, nil
}

func (s *darwinSystem) CleanTemporary(context.Context) (int64, error) { return 0, ErrReadOnly }

// Interfaces lists the hardware ports of networksetup with the addresses Go
// reads. The name of the joined wireless network is not among them: macOS
// keeps it from programs without the permission of Location Services.
func (s *darwinSystem) Interfaces(ctx context.Context) ([]Interface, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "networksetup", "-listallhardwareports").Output()
	if err != nil {
		return nil, err
	}
	var res []Interface
	var port string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "Hardware Port: "); ok {
			port = v
			continue
		}
		dev, ok := strings.CutPrefix(line, "Device: ")
		if !ok {
			continue
		}
		in := Interface{Device: dev, Connection: port, Enabled: true}
		switch {
		case port == "Wi-Fi":
			in.Kind = Wifi
		case strings.Contains(port, "Ethernet") || strings.Contains(port, "LAN"):
			in.Kind = Ethernet
		default:
			continue
		}
		if ni, err := net.InterfaceByName(dev); err == nil {
			in.MAC = ni.HardwareAddr.String()
			addrs, _ := ni.Addrs()
			for _, a := range addrs {
				if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Is4() {
					in.Addr.Address, in.State = p, Connected
				}
			}
			if in.State != Connected {
				in.State = Disconnected
			}
		}
		if info, err := exec.CommandContext(ctx, "networksetup", "-getinfo", port).Output(); err == nil {
			for _, l := range strings.Split(string(info), "\n") {
				switch {
				case strings.HasPrefix(l, "Manual Configuration"):
					in.Config.Method, in.Addr.Method = Manual, Manual
				case strings.HasPrefix(l, "Router: "):
					in.Addr.Router, _ = netip.ParseAddr(strings.TrimPrefix(l, "Router: "))
				}
			}
		}
		res = append(res, in)
	}
	return res, nil
}

func (s *darwinSystem) SetWifi(context.Context, bool) error                 { return ErrReadOnly }
func (s *darwinSystem) Scan(context.Context, string) ([]AccessPoint, error) { return nil, ErrReadOnly }
func (s *darwinSystem) Join(context.Context, string, Join) error            { return ErrReadOnly }
func (s *darwinSystem) Forget(context.Context, string) error                { return ErrReadOnly }
func (s *darwinSystem) Configure(context.Context, string, IPConfig) (Change, error) {
	return nil, ErrReadOnly
}
func (s *darwinSystem) CheckUpdates(context.Context) ([]Package, error)      { return nil, ErrReadOnly }
func (s *darwinSystem) InstallUpdates(context.Context, func(Progress)) error { return ErrReadOnly }
func (s *darwinSystem) Release(context.Context) (Release, error)             { return Release{}, ErrReadOnly }
func (s *darwinSystem) UpgradeRelease(context.Context, func(Progress)) error { return ErrReadOnly }
func (s *darwinSystem) Reboot(context.Context) error                         { return ErrReadOnly }
func (s *darwinSystem) PowerOff(context.Context) error                       { return ErrReadOnly }
func (s *darwinSystem) Install(context.Context, Installation) error          { return ErrReadOnly }
func (s *darwinSystem) SetBoot(context.Context, Boot, bool) error            { return ErrReadOnly }
func (s *darwinSystem) Display(context.Context) (Display, error)             { return Display{}, ErrUnsupported }
func (s *darwinSystem) SetBrightness(context.Context, float64) error         { return ErrReadOnly }
func (s *darwinSystem) SetRotation(context.Context, Rotation) error          { return ErrReadOnly }
func (s *darwinSystem) TimeZones(context.Context) ([]string, error)          { return nil, ErrReadOnly }
func (s *darwinSystem) SetTimeZone(context.Context, string) error            { return ErrReadOnly }
func (s *darwinSystem) SetNTP(context.Context, bool) error                   { return ErrReadOnly }

func (s *darwinSystem) Startup(context.Context) (Startup, error) {
	return Startup{Desktop: true}, nil
}

func (s *darwinSystem) Clock(context.Context) (Clock, error) {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return Clock{}, err
	}
	zone := target
	if i := strings.Index(target, "zoneinfo/"); i >= 0 {
		zone = target[i+len("zoneinfo/"):]
	}
	return Clock{Zone: filepath.ToSlash(zone), NTP: true}, nil
}
