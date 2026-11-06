//go:build darwin && !ios

package ebiten

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
)

// The macOS drag layer; see drop.go and the project plan, section 48.
//
// Ebitengine's view class, GLFWContentView, implements draggingEntered:,
// answering Generic to everything, and performDragOperation:, which reads file
// URLs and answers YES. It has no draggingUpdated:, draggingExited:,
// concludeDragOperation: or draggingEnded:, and this layer adds exactly those
// four to the class with class_addMethod, which leaves Ebitengine's own two
// alone. The class is looked up by name and never through object_getClass of
// the view: key-value observation swaps the class of the live view for a
// subclass, which inherits what is added to GLFWContentView.
//
// AppKit sends draggingUpdated: periodically while a drag is over the window,
// moving or not, so the first one after a quiet spell opens the session. Its
// answer is gift's from the previous update, because the tree lives on the UI
// goroutine and AppKit wants the answer now: Copy when the target under the
// drag accepts it and the source allows a copy, None otherwise.

const (
	nsDragOperationNone = 0
	nsDragOperationCopy = 1

	utiFileURL   = "public.file-url"
	utiURL       = "public.url"
	utiPlainText = "public.utf8-plain-text"
)

type nsPoint struct{ X, Y float64 }

type nsRect struct {
	Origin nsPoint
	Size   struct{ W, H float64 }
}

type macDrops struct {
	dropQueue

	view objc.ID
	log  *slog.Logger

	// inSession is touched on the main thread only.
	inSession bool

	regOnce sync.Once

	sel struct {
		pasteboard, location, opMask, types, containsObject, readObjects, count, objectAtIndex,
		isFileURL, fsRep, absoluteString, utf8, stringForType, convertPoint, bounds, isFlipped,
		window, backingScale, register, arrayWithObjects, alloc, initWithBytes, release objc.SEL
	}
	cls struct{ array, url, str objc.ID }
	uti struct{ fileURL, url, text objc.ID }
}

func openNativeDrops(wake func(), log *slog.Logger) nativeDrops {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		return nil
	}
	m := &macDrops{log: log}
	m.wake = wake
	m.evs = make([]nativeDropEvent, 0, 8)
	r := objc.RegisterName
	s := &m.sel
	s.pasteboard, s.location, s.opMask = r("draggingPasteboard"), r("draggingLocation"), r("draggingSourceOperationMask")
	s.types, s.containsObject, s.readObjects = r("types"), r("containsObject:"), r("readObjectsForClasses:options:")
	s.count, s.objectAtIndex, s.isFileURL = r("count"), r("objectAtIndex:"), r("isFileURL")
	s.fsRep, s.absoluteString, s.utf8 = r("fileSystemRepresentation"), r("absoluteString"), r("UTF8String")
	s.stringForType, s.convertPoint, s.bounds = r("stringForType:"), r("convertPoint:fromView:"), r("bounds")
	s.isFlipped, s.window, s.backingScale = r("isFlipped"), r("window"), r("backingScaleFactor")
	s.register, s.arrayWithObjects = r("registerForDraggedTypes:"), r("arrayWithObjects:count:")
	s.alloc, s.initWithBytes, s.release = r("alloc"), r("initWithBytes:length:encoding:"), r("release")
	m.cls.array, m.cls.url, m.cls.str = objc.ID(objc.GetClass("NSArray")), objc.ID(objc.GetClass("NSURL")), objc.ID(objc.GetClass("NSString"))
	if m.cls.array == 0 || m.cls.url == 0 || m.cls.str == 0 {
		return nil
	}
	m.uti.fileURL, m.uti.url, m.uti.text = m.newString(utiFileURL), m.newString(utiURL), m.newString(utiPlainText)

	cls := objc.GetClass("GLFWContentView")
	if cls == 0 {
		m.debug("gift/backend/ebiten: no GLFWContentView class, the macOS drag layer stays off")
		return nil
	}
	m.view = findContentView(cls)
	if m.view == 0 {
		m.debug("gift/backend/ebiten: no window with Ebitengine's content view, the macOS drag layer stays off")
		return nil
	}
	if !installDragMethods(cls) {
		m.debug("gift/backend/ebiten: GLFWContentView already has its own drag methods, the macOS drag layer stays off")
		return nil
	}
	activeMacDrops.Store(m)
	return m
}

// activeMacDrops is the layer the methods added to the class report to. The
// class is process wide and so is this; there is one window.
var activeMacDrops atomic.Pointer[macDrops]

func currentMacDrops() *macDrops { return activeMacDrops.Load() }

// installOnce guards the class: methods are added once per process even when
// a test opens a second layer.
var (
	installOnce sync.Once
	installed   bool
)

func installDragMethods(cls objc.Class) bool {
	installOnce.Do(func() {
		updated := objc.NewIMP(func(_ objc.ID, _ objc.SEL, info objc.ID) uintptr {
			if m := currentMacDrops(); m != nil {
				return m.updated(info)
			}
			return nsDragOperationNone
		})
		exited := objc.NewIMP(func(_ objc.ID, _ objc.SEL, _ objc.ID) {
			if m := currentMacDrops(); m != nil {
				m.ended()
			}
		})
		conclude := objc.NewIMP(func(_ objc.ID, _ objc.SEL, info objc.ID) {
			if m := currentMacDrops(); m != nil {
				m.concluded(info)
			}
		})
		ended := objc.NewIMP(func(_ objc.ID, _ objc.SEL, _ objc.ID) {
			if m := currentMacDrops(); m != nil {
				m.ended()
			}
		})
		r := objc.RegisterName
		if !cls.AddMethod(r("draggingUpdated:"), updated, "Q@:@") {
			return
		}
		cls.AddMethod(r("draggingExited:"), exited, "v@:@")
		cls.AddMethod(r("concludeDragOperation:"), conclude, "v@:@")
		cls.AddMethod(r("draggingEnded:"), ended, "v@:@")
		installed = true
	})
	return installed
}

// findContentView is the content view of the first window of NSApp that is
// one of Ebitengine's.
func findContentView(cls objc.Class) objc.ID {
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	if app == 0 {
		return 0
	}
	wins := app.Send(objc.RegisterName("windows"))
	n := int(objc.Send[uint64](wins, objc.RegisterName("count")))
	for i := range n {
		w := wins.Send(objc.RegisterName("objectAtIndex:"), uint64(i))
		v := w.Send(objc.RegisterName("contentView"))
		if v != 0 && objc.Send[bool](v, objc.RegisterName("isKindOfClass:"), objc.ID(cls)) {
			return v
		}
	}
	return 0
}

func (m *macDrops) debug(msg string) {
	if m.log != nil {
		m.log.Debug(msg)
	}
}

// answer also registers the full list of dragged types, once the window has
// a target: before that Ebitengine's draggingEntered: would show a text drag
// as accepted by a window that takes nothing.
func (m *macDrops) answer(accepted, hasTargets bool) {
	m.dropQueue.answer(accepted, hasTargets)
	if hasTargets {
		m.regOnce.Do(func() {
			// From the update, which is not the main thread in gift's
			// multi-threaded mode; see installDrops.
			eb.RunOnMainThread(func() {
				m.view.Send(m.sel.register, m.array(m.uti.fileURL, m.uti.url, m.uti.text))
			})
		})
	}
}

func (m *macDrops) close() { activeMacDrops.CompareAndSwap(m, nil) }

// updated is draggingUpdated:.
func (m *macDrops) updated(info objc.ID) uintptr {
	if !m.inSession {
		m.inSession = true
		m.open()
	}
	x, y := m.position(info)
	m.push(nativeDropEvent{op: nativeOver, x: x, y: y, kinds: m.kinds(info.Send(m.sel.pasteboard))})
	if m.wouldAccept() && objc.Send[uint64](info, m.sel.opMask)&nsDragOperationCopy != 0 {
		return nsDragOperationCopy
	}
	return nsDragOperationNone
}

// ended is draggingExited: and draggingEnded:.
func (m *macDrops) ended() {
	if !m.inSession {
		return
	}
	m.inSession = false
	m.push(nativeDropEvent{op: nativeLeave})
}

// concluded is concludeDragOperation:, which follows Ebitengine's
// performDragOperation: because that one always answers YES.
func (m *macDrops) concluded(info objc.ID) {
	x, y := m.position(info)
	m.push(nativeDropEvent{op: nativeDrop, x: x, y: y, data: m.read(info.Send(m.sel.pasteboard))})
}

// position is the drag location in the unit of Ebitengine's cursor: points
// from the top left corner of the view, times the backing scale.
func (m *macDrops) position(info objc.ID) (float64, float64) {
	p := objc.Send[nsPoint](info, m.sel.location)
	p = objc.Send[nsPoint](m.view, m.sel.convertPoint, p, objc.ID(0))
	if !objc.Send[bool](m.view, m.sel.isFlipped) {
		b := objc.Send[nsRect](m.view, m.sel.bounds)
		p.Y = b.Size.H - p.Y
	}
	scale := 1.0
	if w := m.view.Send(m.sel.window); w != 0 {
		if s := objc.Send[float64](w, m.sel.backingScale); s > 0 {
			scale = s
		}
	}
	return p.X * scale, p.Y * scale
}

// kinds is what the pasteboard offers, by the precedence of [dropPrecedence]:
// a file URL makes it files, whatever else a Finder drag carries.
func (m *macDrops) kinds(pb objc.ID) gift.DropKind {
	types := pb.Send(m.sel.types)
	if types == 0 {
		return 0
	}
	has := func(uti objc.ID) bool { return objc.Send[bool](types, m.sel.containsObject, uti) }
	switch {
	case has(m.uti.fileURL):
		return gift.DropFile
	case has(m.uti.url):
		k := gift.DropURL
		if has(m.uti.text) {
			if t, ok := m.text(pb); ok && !sameAsURLs(t, m.urls(pb)) {
				k |= gift.DropText
			}
		}
		return k
	case has(m.uti.text):
		return gift.DropText
	}
	return 0
}

// read is the dropped content.
func (m *macDrops) read(pb objc.ID) gift.DropData {
	var files, addrs []string
	urls := pb.Send(m.sel.readObjects, m.array(m.cls.url), objc.ID(0))
	if urls != 0 {
		n := int(objc.Send[uint64](urls, m.sel.count))
		for i := range n {
			u := urls.Send(m.sel.objectAtIndex, uint64(i))
			if objc.Send[bool](u, m.sel.isFileURL) {
				// fileSystemRepresentation, as Ebitengine does, because the
				// Finder hands out file reference URLs whose path is not the
				// path.
				if p := goCString(objc.Send[unsafe.Pointer](u, m.sel.fsRep)); p != "" {
					files = append(files, p)
				}
				continue
			}
			if s := m.goString(u.Send(m.sel.absoluteString)); s != "" {
				addrs = append(addrs, s)
			}
		}
	}
	text, hasText := m.text(pb)
	return dropPrecedence(files, addrs, text, hasText)
}

// urls are the addresses on the pasteboard that are not files.
func (m *macDrops) urls(pb objc.ID) []string {
	var out []string
	urls := pb.Send(m.sel.readObjects, m.array(m.cls.url), objc.ID(0))
	if urls == 0 {
		return nil
	}
	n := int(objc.Send[uint64](urls, m.sel.count))
	for i := range n {
		u := urls.Send(m.sel.objectAtIndex, uint64(i))
		if !objc.Send[bool](u, m.sel.isFileURL) {
			out = append(out, m.goString(u.Send(m.sel.absoluteString)))
		}
	}
	return out
}

func (m *macDrops) text(pb objc.ID) (string, bool) {
	s := pb.Send(m.sel.stringForType, m.uti.text)
	if s == 0 {
		return "", false
	}
	return m.goString(s), true
}

func (m *macDrops) goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	return goCString(objc.Send[unsafe.Pointer](s, m.sel.utf8))
}

func goCString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// newString is a retained NSString of s.
func (m *macDrops) newString(s string) objc.ID {
	b := []byte(s)
	if len(b) == 0 {
		b = []byte{0}
	}
	const nsUTF8StringEncoding = 4
	return m.cls.str.Send(m.sel.alloc).Send(m.sel.initWithBytes, unsafe.Pointer(&b[0]), uint64(len(s)), uint64(nsUTF8StringEncoding))
}

func (m *macDrops) array(objs ...objc.ID) objc.ID {
	return m.cls.array.Send(m.sel.arrayWithObjects, unsafe.Pointer(&objs[0]), uint64(len(objs)))
}
