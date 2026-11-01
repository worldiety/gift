// Command giftbundle runs a Go program as a macOS app bundle. It exists for
// gift/notify: macOS shows notifications only for an application that is a
// signed bundle known to LaunchServices, and a binary built by `go run` or
// `go test` is none of that.
//
// It is meant to sit behind the -exec flag of the go command, which calls it
// with the freshly built binary and the program's arguments:
//
//	go install github.com/worldiety/gift/cmd/giftbundle@latest
//	go run -exec "giftbundle -id com.example.Photos" ./cmd/photos
//	GIFT_NOTIFY_MACOS=1 go test -exec giftbundle ./notify/ -run MacOS -v
//
// # What it does
//
//  1. Copies the binary into a bundle at a fixed place,
//     ~/Library/Application Support/gift/bundles/<id>.app, so that the
//     permission the user grants once stays with the bundle across rebuilds.
//     Not next to the binary: the go command builds under /var/folders, and
//     macOS refuses notifications to a bundle there.
//  2. Writes Contents/Info.plist with the identifier, the name and
//     NSHighResolutionCapable, without which a bundled window renders at half
//     resolution on a Retina display.
//  3. Signs the bundle ad hoc, `codesign --force --sign -`. The signature the
//     Go linker gives the binary covers the binary and not the bundle.
//  4. Registers it with LaunchServices, `lsregister -f`.
//  5. Replaces itself with the bundled binary, so that standard input and
//     output, signals and the exit status are the program's own.
//
// On any other operating system it runs the binary unchanged, so that a
// shared GOFLAGS=-exec=giftbundle does no harm there.
//
// The bundle is for development. It is not notarised and carries no
// Developer ID signature, which an application handed to other people needs
// for Gatekeeper; notifications do not.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "giftbundle:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("giftbundle", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: giftbundle [-id identifier] [-name name] [-icon file.icns] binary [arguments...]")
		fs.PrintDefaults()
	}
	id := fs.String("id", "", "CFBundleIdentifier; default dev.gift.<binary name>")
	name := fs.String("name", "", "CFBundleName, shown as the sender of notifications; default the binary name")
	icon := fs.String("icon", "", "an .icns file for the bundle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("no binary given")
	}
	bin, progArgs := fs.Arg(0), fs.Args()[1:]

	if runtime.GOOS != "darwin" {
		return runPlain(bin, progArgs)
	}

	spec := bundleSpec{ID: *id, Name: *name, Icon: *icon}
	spec.fill(filepath.Base(bin))
	if err := spec.validate(); err != nil {
		return err
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(base, "gift", "bundles", spec.ID+".app")
	exe, err := spec.write(dir, bin)
	if err != nil {
		return err
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", dir).CombinedOutput(); err != nil {
		return fmt.Errorf("signing %s: %v\n%s", dir, err, out)
	}
	if out, err := exec.Command(lsregister, "-f", dir).CombinedOutput(); err != nil {
		// Not fatal: launching from the bundle registers it as well, and
		// the program reports a refusal itself.
		fmt.Fprintf(os.Stderr, "giftbundle: registering %s: %v\n%s", dir, err, out)
	}
	return execProgram(exe, progArgs)
}

// lsregister is the LaunchServices tool. It is not on the PATH, and its place
// has not changed since Mac OS X 10.5.
const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// runPlain runs bin as a child and exits with its status. A child rather than
// exec, because Windows has no exec.
func runPlain(bin string, args []string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		os.Exit(ee.ExitCode())
	}
	return err
}
