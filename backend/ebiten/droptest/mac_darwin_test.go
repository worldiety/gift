//go:build darwin && !ios

package droptest

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
)

// The macOS test drives the real AppKit view of the window with a drag that
// is simulated but otherwise complete: an NSDraggingInfo of its own, an
// NSPasteboard of its own, and the methods called in the order AppKit calls
// them — Ebitengine's own draggingEntered: and performDragOperation: as well
// as the four gift added.

type nsPoint struct{ X, Y float64 }

// fakeInfo is what the registered GiftFakeDraggingInfo answers. It is set on
// the main thread before each call and read there.
var fakeInfo struct {
	pb  objc.ID
	loc nsPoint
	ops uint64
}

var (
	fakeOnce  sync.Once
	fakeClass objc.Class
)

func registerFakeInfo(t *testing.T) objc.ID {
	t.Helper()
	fakeOnce.Do(func() {
		c, err := objc.RegisterClass("GiftFakeDraggingInfo", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: objc.RegisterName("draggingPasteboard"), Fn: func(objc.ID, objc.SEL) objc.ID { return fakeInfo.pb }},
			{Cmd: objc.RegisterName("draggingLocation"), Fn: func(objc.ID, objc.SEL) nsPoint { return fakeInfo.loc }},
			{Cmd: objc.RegisterName("draggingSourceOperationMask"), Fn: func(objc.ID, objc.SEL) uint64 { return fakeInfo.ops }},
			{Cmd: objc.RegisterName("draggingSequenceNumber"), Fn: func(objc.ID, objc.SEL) int64 { return 1 }},
		})
		if err != nil {
			panic(err)
		}
		fakeClass = c
	})
	var info objc.ID
	onMain(func() { info = objc.ID(fakeClass).Send(objc.RegisterName("new")) })
	return info
}

func onMain(f func()) { eb.RunOnMainThread(f) }

func nsString(s string) objc.ID {
	b := append([]byte(s), 0)
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), unsafe.Pointer(&b[0]))
}

func nsArray(objs ...objc.ID) objc.ID {
	return objc.ID(objc.GetClass("NSArray")).Send(objc.RegisterName("arrayWithObjects:count:"), unsafe.Pointer(&objs[0]), uint64(len(objs)))
}

// content is what a simulated drag carries.
type content struct {
	files []string
	urls  []string
	text  string
}

// pasteboard is a uniquely named pasteboard holding c, built on the main
// thread; release it with releaseGlobally.
func pasteboard(c content) objc.ID {
	pb := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("pasteboardWithUniqueName"))
	pb.Send(objc.RegisterName("clearContents"))
	var objs []objc.ID
	urlClass := objc.ID(objc.GetClass("NSURL"))
	for _, f := range c.files {
		objs = append(objs, urlClass.Send(objc.RegisterName("fileURLWithPath:"), nsString(f)))
	}
	for _, u := range c.urls {
		objs = append(objs, urlClass.Send(objc.RegisterName("URLWithString:"), nsString(u)))
	}
	if len(objs) > 0 {
		pb.Send(objc.RegisterName("writeObjects:"), nsArray(objs...))
	}
	if c.text != "" {
		text := nsString("public.utf8-plain-text")
		pb.Send(objc.RegisterName("addTypes:owner:"), nsArray(text), objc.ID(0))
		pb.Send(objc.RegisterName("setString:forType:"), nsString(c.text), text)
	}
	return pb
}

// macView is gift's window's content view.
func macView(t *testing.T) objc.ID {
	t.Helper()
	var v objc.ID
	deadline := time.Now().Add(10 * time.Second)
	for v == 0 && time.Now().Before(deadline) {
		onMain(func() {
			app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
			wins := app.Send(objc.RegisterName("windows"))
			if objc.Send[uint64](wins, objc.RegisterName("count")) > 0 {
				v = wins.Send(objc.RegisterName("objectAtIndex:"), uint64(0)).Send(objc.RegisterName("contentView"))
			}
		})
		if v == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if v == 0 {
		t.Fatal("no window within 10 s")
	}
	return v
}

// at is the window base location of the logical point (x, y) from the top
// left: AppKit counts from the bottom.
func at(view objc.ID, x, y float64) nsPoint {
	var h float64
	onMain(func() {
		type rect struct{ X, Y, W, H float64 }
		h = objc.Send[rect](view, objc.RegisterName("frame")).H
	})
	return nsPoint{x, h - y}
}

// drag runs one simulated drag of c to the logical point (x, y): entered,
// three updates a frame apart, then either the drop — perform, conclude,
// ended — or, with leave set, an exit. It returns the answers of the updates.
func drag(t *testing.T, view, info objc.ID, c content, x, y float64, leave bool) []uint64 {
	t.Helper()
	var pb objc.ID
	onMain(func() { pb = pasteboard(c) })
	defer onMain(func() { pb.Send(objc.RegisterName("releaseGlobally")) })
	loc := at(view, x, y)
	call := func(sel string) uint64 {
		var r uint64
		onMain(func() {
			fakeInfo.pb, fakeInfo.loc, fakeInfo.ops = pb, loc, 1|2|16 // copy, link, move
			r = objc.Send[uint64](view, objc.RegisterName(sel), info)
		})
		return r
	}
	call("draggingEntered:")
	var answers []uint64
	for range 3 {
		answers = append(answers, call("draggingUpdated:"))
		time.Sleep(60 * time.Millisecond)
	}
	if leave {
		call("draggingExited:")
		return answers
	}
	call("performDragOperation:")
	call("concludeDragOperation:")
	call("draggingEnded:")
	return answers
}

// quiet fails when another drop arrives within d.
func quiet(t *testing.T, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case e := <-events:
			if e.targeted == nil {
				t.Errorf("an extra drop arrived at %q: %+v", e.area, e.info)
			}
		case <-timeout:
			return
		}
	}
}

func macFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	a := filepath.Join(dir, "Grüße a.txt")
	folder := filepath.Join(dir, "Ordner")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	// The real path, as the Finder reports it: /private/var, not /var.
	a, _ = filepath.EvalSymlinks(a)
	folder, _ = filepath.EvalSymlinks(folder)
	return a, folder
}

func TestMacNativeLayer(t *testing.T) {
	need(t, "GIFT_DROP_MACOS")
	if os.Getenv("GIFT_DROP_NATIVE") == "0" {
		t.Skip("the native layer is switched off; see TestMacEbitenPath")
	}
	c := waitCaps(t, gift.DropFile|gift.DropText|gift.DropURL)
	if !c.Live {
		t.Errorf("capabilities %+v, want a live session", c)
	}
	view := macView(t)
	info := registerFakeInfo(t)

	t.Run("methods", func(t *testing.T) {
		for _, sel := range []string{"draggingUpdated:", "draggingExited:", "concludeDragOperation:", "draggingEnded:"} {
			var ok bool
			onMain(func() { ok = objc.Send[bool](view, objc.RegisterName("respondsToSelector:"), objc.RegisterName(sel)) })
			if !ok {
				t.Errorf("the view does not respond to %s", sel)
			}
		}
		deadline := time.Now().Add(5 * time.Second)
		var types []string
		for time.Now().Before(deadline) {
			types = types[:0]
			onMain(func() {
				arr := view.Send(objc.RegisterName("registeredDraggedTypes"))
				n := objc.Send[uint64](arr, objc.RegisterName("count"))
				for i := range n {
					s := arr.Send(objc.RegisterName("objectAtIndex:"), i)
					types = append(types, goString(s))
				}
			})
			if slices.Contains(types, "public.file-url") && slices.Contains(types, "public.utf8-plain-text") && slices.Contains(types, "public.url") {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("registeredDraggedTypes %q, want the file URL, the URL and the text UTI", types)
	})

	t.Run("files", func(t *testing.T) {
		a, folder := macFiles(t)
		drain()
		answers := drag(t, view, info, content{files: []string{a, folder}}, 300, 200, false)
		if answers[len(answers)-1] != 1 {
			t.Errorf("draggingUpdated: answered %v, want copy (1) once gift has seen the drag", answers)
		}
		e := next(t, true)
		if e.area != "outer" || e.targeted == nil || !*e.targeted {
			t.Fatalf("first event %+v, want the outer destination targeted", e)
		}
		e = next(t, false)
		if e.area != "outer" {
			t.Fatalf("the drop went to %q", e.area)
		}
		if got := e.info.Files(); !samePaths(got, []string{a, folder}) {
			t.Errorf("files %q, want %q", got, []string{a, folder})
		}
		if !e.info.Items[1].Dir {
			t.Error("the folder arrived as a file")
		}
		if l := e.info.Location; l.X < 295 || l.X > 305 || l.Y < 195 || l.Y > 205 {
			t.Errorf("Location %v, want (300, 200)", l)
		}
		quiet(t, 400*time.Millisecond)
	})

	t.Run("finder", func(t *testing.T) {
		a, _ := macFiles(t)
		drain()
		// Over the text destination: the paths as text must not take the
		// drop away from the file destination.
		drag(t, view, info, content{files: []string{a}, text: a}, 50, 50, false)
		e := next(t, false)
		if e.area != "outer" || len(e.info.Items) != 1 || !samePaths([]string{e.info.Items[0].Path}, []string{a}) {
			t.Errorf("a Finder drag delivered %q %+v", e.area, e.info.Items)
		}
		quiet(t, 400*time.Millisecond)
	})

	t.Run("text", func(t *testing.T) {
		drain()
		drag(t, view, info, content{text: "Hallo Welt"}, 50, 50, false)
		e := next(t, false)
		if e.area != "inner" || len(e.info.Items) != 1 || e.info.Items[0].Text != "Hallo Welt" {
			t.Errorf("text delivered %q %+v", e.area, e.info.Items)
		}
	})

	t.Run("url", func(t *testing.T) {
		drain()
		const u = "https://example.org/a?b=c"
		drag(t, view, info, content{urls: []string{u}, text: u}, 50, 50, false)
		e := next(t, false)
		if e.area != "inner" || len(e.info.Items) != 1 || e.info.Items[0].Kind != gift.DropURL || e.info.Items[0].URL != u {
			t.Errorf("an address delivered %q %+v", e.area, e.info.Items)
		}
	})

	t.Run("exit", func(t *testing.T) {
		drain()
		drag(t, view, info, content{text: "x"}, 50, 50, true)
		on := next(t, true)
		off := next(t, true)
		if on.targeted == nil || !*on.targeted || off.targeted == nil || *off.targeted || on.area != "inner" || off.area != "inner" {
			t.Errorf("events %+v then %+v, want inner targeted true then false", on, off)
		}
		quiet(t, 300*time.Millisecond)
	})

	t.Run("refused", func(t *testing.T) {
		drain()
		// Text over the part of the window only the file destination
		// covers: refused, and the system is told so.
		answers := drag(t, view, info, content{text: "x"}, 300, 200, true)
		if answers[len(answers)-1] != 0 {
			t.Errorf("draggingUpdated: answered %v, want none (0) for text nobody there takes", answers)
		}
	})
}

// TestMacEbitenPath is the drop without the native layer, through
// Ebitengine's performDragOperation: alone:
//
//	GIFT_DROP_MACOS=1 GIFT_DROP_NATIVE=0 go test ./backend/ebiten/droptest -run MacEbitenPath -v -count=1
func TestMacEbitenPath(t *testing.T) {
	need(t, "GIFT_DROP_MACOS")
	if os.Getenv("GIFT_DROP_NATIVE") != "0" {
		t.Skip("set GIFT_DROP_NATIVE=0 as well")
	}
	if c := waitCaps(t, gift.DropFile); c.Live || c.Kinds != gift.DropFile {
		t.Errorf("capabilities %+v, want files without a live session", c)
	}
	view := macView(t)
	info := registerFakeInfo(t)
	a, _ := macFiles(t)
	drain()
	var pb objc.ID
	onMain(func() { pb = pasteboard(content{files: []string{a}}) })
	loc := at(view, 300, 200)
	onMain(func() {
		fakeInfo.pb, fakeInfo.loc, fakeInfo.ops = pb, loc, 1
		objc.Send[uint64](view, objc.RegisterName("draggingEntered:"), info)
		objc.Send[bool](view, objc.RegisterName("performDragOperation:"), info)
	})
	e := next(t, false)
	if e.area != "outer" || !samePaths(e.info.Files(), []string{a}) {
		t.Errorf("Ebitengine's drop delivered %q %+v", e.area, e.info.Items)
	}
	onMain(func() { pb.Send(objc.RegisterName("releaseGlobally")) })
}

func goString(s objc.ID) string {
	p := objc.Send[unsafe.Pointer](s, objc.RegisterName("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// samePaths reports whether got and want name the same files. The bytes may
// differ: fileSystemRepresentation decomposes an umlaut (NFD), as the file
// system stores it, where Go wrote it composed; both open the same file.
func samePaths(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		a, errA := os.Stat(got[i])
		b, errB := os.Stat(want[i])
		if errA != nil || errB != nil || !os.SameFile(a, b) {
			return false
		}
	}
	return true
}
