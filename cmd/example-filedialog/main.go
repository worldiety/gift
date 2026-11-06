// Command example-filedialog opens the operating system's own file dialog
// with gift/filedialog: a button per mode, and a line with what was picked.
//
//	go run ./cmd/example-filedialog
//
// On macOS the panel hangs on the window as a sheet. A switch chooses the
// system's dialog, the in-app picker of ui/filepicker, or the first with the
// second as its fallback; both report paths relative to the home directory,
// so one pick function serves them. The extensions field limits Open and
// Open several. Where there is no native dialog, "Native" answers at once
// with "cancelled", and the status line says that nothing can be shown.
//
// With GIFT_FILEDIALOG_SELFTEST=1 the program tests itself instead of waiting
// for clicks: it opens a dialog in each mode, counts the updates of the
// application for one second while the dialog is up, closes it from code,
// prints the count per mode and exits with 0 when every count is at least
// 20, and with 1 otherwise. A dialog that held up the main thread — a modal
// loop of its own — would stop the updates and fail it.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/filedialog"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/filepicker"
)

// modes are the buttons, in their order.
var modes = []struct {
	label string
	mode  filedialog.Mode
}{
	{"Open", filedialog.Open},
	{"Open several", filedialog.OpenMulti},
	{"Choose a folder", filedialog.Directory},
	{"Save", filedialog.Save},
}

// model is what the window shows. It is touched only on the UI executor: in
// the handlers of the window, in the callbacks of the dialog, which arrive
// through Post, and in OnUpdate.
type model struct {
	mode    filedialog.Mode
	picking bool
	// way is 0 for the system's dialog, 1 for the in-app picker and 2 for
	// the system's dialog with the in-app picker as fallback.
	way    int
	result string
	// answers counts the callbacks of the dialog, for the self-test.
	answers int
}

var (
	app    *gift.App
	logger *slog.Logger
	m      = model{result: "Nothing picked yet."}
)

func main() {
	if err := example.LoadFont(); err != nil {
		fmt.Fprintln(os.Stderr, "example-filedialog:", err)
		os.Exit(1)
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app = gift.New(gift.Options{Root: root})
	cfg := backend.Config{Title: "gift file dialog", Width: 640, Height: 480, Logger: logger,
		X11ClassName: "dev.gift.example-filedialog", X11InstanceName: "example-filedialog"}
	var st *selfTest
	if os.Getenv("GIFT_FILEDIALOG_SELFTEST") == "1" {
		st = &selfTest{}
		cfg.OnUpdate = st.update
	}
	if err := backend.Run(app, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "example-filedialog:", err)
		os.Exit(1)
	}
	if st != nil {
		os.Exit(st.report())
	}
}

func pick(paths []string) {
	m.picking = false
	m.answers++
	m.result = "Picked: " + strings.Join(paths, ", ")
	app.Invalidate()
}

func cancel() {
	m.picking = false
	m.answers++
	m.result = "cancelled"
	app.Invalidate()
}

func open(mode filedialog.Mode) {
	m.mode, m.picking = mode, true
	app.Invalidate()
}

func root(ctx *gift.Context) gift.View {
	pad := geom.Insets{Top: 8, Right: 16, Bottom: 8, Left: 16}
	buttons := make([]gift.View, len(modes))
	for i, md := range modes {
		buttons[i] = ui.Button(ui.Text(md.label), func() { open(md.mode) }).
			CornerRadius(10).PaddingInsets(pad).MinHeight(ui.ControlHitTarget)
	}
	status := "The system's file dialog is available."
	if !filedialog.Available() {
		status = "No native file dialog here: Native answers with cancelled."
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	extField := ui.Editor(ctx, "exts", ".jpg, .png")
	screen := ui.VStack(
		ui.Text("File dialog").FontSize(24),
		ui.SegmentedControl(m.way, []string{"Native", "In-app", "Native, else in-app"}, func(i int) {
			m.way = i
			app.Invalidate()
		}),
		ui.HStack(buttons...).Gap(8),
		ui.TextField(extField).Placeholder("Extensions for Open, such as .jpg, .png").Frame(420, geom.Unbounded()),
		ui.Text(m.result).FontSize(15),
		ui.Text(status+" Paths are relative to "+filepath.ToSlash(home)+".").FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(16).Padding(24)

	var exts []string
	if m.mode == filedialog.Open || m.mode == filedialog.OpenMulti {
		for _, e := range strings.Split(extField.Text(), ",") {
			if e = strings.TrimSpace(e); e != "" {
				exts = append(exts, e)
			}
		}
	}
	dlg := filedialog.New(pick, cancel).Mode(m.mode).Logger(logger).Root(home).Extensions(exts...)
	picker := filepicker.New(os.DirFS(home), pick, cancel).Mode(m.mode).Logger(logger).Extensions(exts...)
	if m.mode == filedialog.Save {
		dlg = dlg.SuggestedName("Untitled.txt").Extensions(".txt")
		picker = picker.SuggestedName("Untitled.txt")
	}
	var v gift.View
	switch m.way {
	case 1:
		v = ui.Sheet(screen, picker).Presented(m.picking).OnDismiss(cancel)
	case 2:
		v = filedialog.Present(screen, dlg).Fallback(picker).Presented(m.picking)
	default:
		v = filedialog.Present(screen, dlg).Presented(m.picking)
	}
	return ui.Window(v).Align(geom.Alignment{X: 0.5, Y: 0.5})
}

// The phases of one mode in the self-test.
const (
	settle  = 500 * time.Millisecond // from the open until counting starts
	measure = time.Second            // counting while the dialog is up
	rest    = 500 * time.Millisecond // after the close, for the system to confirm it
	minimum = 20                     // updates in measure that pass
)

// selfTest opens a dialog per mode from OnUpdate and counts updates.
type selfTest struct {
	i      int // index into modes
	phase  int // 0 open, 1 settling, 2 counting, 3 resting
	since  time.Time
	count  int
	counts []int
	failed []string
}

func (s *selfTest) update() error {
	now := time.Now()
	switch s.phase {
	case 0:
		if s.i == len(modes) {
			return backend.Terminate
		}
		m.answers = 0
		open(modes[s.i].mode)
		s.phase, s.since, s.count = 1, now, 0
	case 1:
		if now.Sub(s.since) >= settle {
			s.phase, s.since = 2, now
		}
	case 2:
		s.count++
		if now.Sub(s.since) >= measure {
			s.counts = append(s.counts, s.count)
			if m.answers != 0 {
				s.failed = append(s.failed, fmt.Sprintf("%s answered %q before it was closed", modes[s.i].label, m.result))
			}
			// Closing from code: Presented(false) closes the dialog
			// without a callback.
			m.picking = false
			app.Invalidate()
			s.phase, s.since = 3, now
		}
	case 3:
		if now.Sub(s.since) >= rest {
			if m.answers != 0 {
				s.failed = append(s.failed, fmt.Sprintf("%s answered %q after it was closed from code", modes[s.i].label, m.result))
			}
			s.i++
			s.phase = 0
		}
	}
	return nil
}

// report prints the counts and returns the exit code.
func (s *selfTest) report() int {
	code := 0
	if !filedialog.Available() {
		fmt.Println("selftest: no native file dialog on this system")
		code = 1
	}
	for i, c := range s.counts {
		verdict := "ok"
		if c < minimum {
			verdict = fmt.Sprintf("FAIL, want at least %d", minimum)
			code = 1
		}
		fmt.Printf("selftest: %-16s %3d updates in %v with the dialog up: %s\n", modes[i].label, c, measure, verdict)
	}
	if len(s.counts) != len(modes) {
		fmt.Printf("selftest: only %d of %d modes ran\n", len(s.counts), len(modes))
		code = 1
	}
	for _, f := range s.failed {
		fmt.Println("selftest: FAIL,", f)
		code = 1
	}
	return code
}
