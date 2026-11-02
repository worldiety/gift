//go:build darwin && !ios

package tray

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// This file is the macOS half: an NSStatusItem with an NSMenu, through the
// Objective-C runtime, the calls an AppKit program would make.
//
//	[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength]
//	[[item button] setImage:]  [[item button] setToolTip:]
//	[item setMenu:]                                   opens on a left and a right click
//	[[NSStatusBar systemStatusBar] removeStatusItem:]
//
// A status item with a menu opens that menu on a click of either button,
// which is exactly the behaviour the package promises, so there is no event
// handling of the icon itself here at all.
//
// # The main thread, and only through the main queue
//
// AppKit may be touched only on the main thread, and the main thread belongs
// to Ebitengine: it sits in GLFW's event loop, which asks NSApplication for
// the next event and so runs the main run loop. Set, on the other hand, is
// called from the UI executor, which is Update on Ebitengine's game thread,
// or from any goroutine at all.
//
// So update and remove do no AppKit work. They prepare what they can in Go —
// the pictures are rasterised and encoded on the caller's goroutine, not on
// the main thread — store the result as the wanted state of the icon, and
// hand a function to the main dispatch queue with dispatch_async_f. The run
// loop of the main thread serves that queue in its common modes, which
// include the event tracking mode an open menu runs in, so the change
// arrives within the next turn of the loop. dispatch_sync is never used: a
// caller that is itself on the main thread, which Ebitengine's single
// threaded mode makes of Update, would wait for itself for ever.
//
// The function crosses into C as a purego callback, which is a resource the
// process gets about two thousand of and never gives back. There is exactly
// one, made when the package first opens, and what it runs is looked up in a
// table under the context word dispatch_async_f passes along: a number, not a
// Go pointer, which C may not hold.
//
// Several updates before the main thread comes round collapse into one. A
// tray keeps the latest wanted state and a flag that a function is already
// queued; a second update only replaces the state. The main thread applies
// whatever is wanted when it runs, so a burst of Sets — a check mark toggled
// in a loop — costs one rebuild of the menu. SetIcon and SetTooltip cost
// none: they hand on the menu of the last Set unchanged, and the NSMenu built
// from it stays where it is (see "Clicks").
//
// # No NSApplication, no tray — yet
//
// A status item exists only for a process with an NSApplication whose run
// loop runs: without one the item is accepted, never drawn, and a click on
// it, if there were one, would never be delivered. A command line tool and a
// test binary are such processes, and they get [ErrUnsupported].
//
// But "no NSApplication" is also what a gift application looks like before
// backend.Run: Ebitengine creates NSApp when it initialises GLFW, inside
// RunGame. A Set in main before Run, which is how the package example reads,
// must not leave the tray dead for its whole life, and the portable half
// opens a platform only once. So the question is not asked when the platform
// opens, but at every update and on the main thread:
//
//   - update without NSApp keeps the state and queues it for the main thread
//     as always, and returns an error that wraps [ErrUnsupported] and says
//     the icon appears once the application runs. A command line tool gets
//     the error the package promises; an application that set its menu too
//     early has it shown as soon as Ebitengine's run loop serves the main
//     queue, which it does from its first turn.
//   - apply, on the main thread, asks again before it touches the status
//     bar. A main run loop without NSApp — a tool that runs CFRunLoop itself
//     — logs once and leaves the state wanted, and the next update tries
//     again.
//
// Nothing piles up meanwhile: an update while one is queued only replaces
// the wanted state.
//
// The question has to be asked without creating the application:
// [NSApplication sharedApplication] would make one, and with it a process
// that suddenly has a menu bar and a Dock icon. AppKit keeps the shared
// application in the global variable NSApp, which is nil until somebody has
// created it, and that variable is read instead, through its address from
// dlsym. Reading it goes through memmove from libSystem rather than through
// a conversion of the address to a pointer, for the reason clipboard_darwin.go
// gives for NSPasteboardTypeString: go vet's unsafeptr check refuses the
// conversion, and is right to, since nothing keeps that address valid in the
// eyes of the compiler.
//
// # Clicks
//
// Every entry that can be clicked has the selector trayClick: as its action
// and as its target an object of a class registered here, GiftTrayTarget,
// one per [Tray]. A menu item does not retain its target; the targets are
// therefore never released, which costs one small object per Tray and makes
// a click in a menu that is still open after it was replaced harmless.
//
// The click has to say which state it was made in, because the portable half
// drops a click in a menu that has been replaced since. The tag of the menu
// item carries both numbers: the id of the entry in the low 32 bits and the
// low 31 bits of the generation above them, which keeps the NSInteger
// positive. The whole generation is recovered from the latest one handed to
// update, which is never 2^31 states ahead of a menu a user can still click
// in; see [fullGen]. A table from menus to generations would need the
// lifetime of the old menu, which AppKit owns while it is open.
//
// The generation in the tags is that of the Set the menu was built from, not
// of the latest state. SetIcon and SetTooltip make a new generation as well,
// and the portable half drops a click whose generation is not the newest; a
// menu that is open while a tooltip counts would go deaf. They pass the menu
// of the last Set on unchanged, the same slice, so a click in a menu built
// at or after the generation of that Set is reported with the latest
// generation (see [menuGens]), as the Windows half does; [sameMenu] is the rule.
// A Set makes a new slice and with it a new menu generation, so a click in
// a menu it replaced is still dropped; so is one after a Remove, which the
// portable half counts without telling the platform.
//
// The action runs on the main thread, in the middle of AppKit's menu
// tracking. It hands the click to cfg.clicked on a goroutine of its own, so
// that neither the lock of the portable half nor Post can ever hold up the
// main thread.
//
// # Pictures
//
// The icon is a template image: black where the symbol is, transparent where
// it is not, and AppKit draws it in the colour of the menu bar — dark on a
// light bar, light on a dark one, highlighted while the menu is open. That is
// why macOS gets the bare coverage mask and not [tinted]. The symbols in the
// menu are template images too, and follow the highlight of their row.
//
// The mask is handed over as a PNG, decoded by [NSImage initWithData:], and
// sized in points with setSize:, which makes a bitmap of twice the points a
// Retina picture. The alternative, NSBitmapImageRep's initializer with ten
// arguments, puts four of them on the stack, where Apple's arm64 convention
// packs arguments by their own size rather than in eight byte slots. purego
// knows that convention, but a call whose layout only one library gets right
// is a call that breaks quietly; a PNG is one pointer and one length. The
// encoder is Go's own and the pictures are a few hundred bytes.
//
// The pixel size is the point size times the largest backing scale factor of
// the screens, read on the main thread when a state is applied and assumed to
// be 2 before that, which is every Mac with a built-in display since 2020.
// A state prepared at a scale that turns out to be wrong is prepared again on
// the main thread; that happens once, when the largest display is not 2x.

// The values from the AppKit headers. Numbers in an ABI.
const (
	// nsVariableStatusItemLength is NSVariableStatusItemLength, a CGFloat:
	// the item is as wide as its button needs.
	nsVariableStatusItemLength = -1.0

	// nsControlStateValueOn is NSControlStateValueOn, the check mark of a
	// menu item.
	nsControlStateValueOn = 1

	nsUTF8StringEncoding = 4
)

// The sizes of the pictures, in points. 18 is what Apple's own template
// images in the menu bar are drawn at, 16 the size of an image in a menu.
const (
	barIconPoints  = 18
	menuIconPoints = 16
)

// nsSize is NSSize (CGSize) on a 64 bit Mac: two CGFloats, passed in two
// floating point registers. tray_darwin_test.go checks the size.
type nsSize struct {
	Width, Height float64
}

// nsRect is NSRect (CGRect): an origin and a size. Only the tests read one.
type nsRect struct {
	X, Y          float64
	Width, Height float64
}

// macSel holds the classes and selectors, resolved once.
type macSel struct {
	poolClass, stringClass, dataClass, imageClass objc.Class
	statusBarClass, menuClass, menuItemClass      objc.Class
	screenClass, objectClass                      objc.Class

	alloc, initSel, retain, release, drain         objc.SEL
	initWithBytes, dataWithBytes, initWithData     objc.SEL
	setSize, setTemplate                           objc.SEL
	systemStatusBar, statusItemWithLength          objc.SEL
	removeStatusItem, button, setImage, setToolTip objc.SEL
	setTitle, setMenu, menu                        objc.SEL
	initWithTitle, initItem, setAutoenables        objc.SEL
	addItem, separatorItem, setTarget, setTag, tag objc.SEL
	setEnabled, setState, setSubmenu, trayClick    objc.SEL
	screens, count, objectAtIndex, backingScale    objc.SEL
	setAccessibilityLabel                          objc.SEL
}

// macState is the process wide part: the frameworks, the one callback, the
// table of work for the main queue and the targets of the clicks.
type macState struct {
	sel macSel

	// nsApp is the address of AppKit's global NSApp, see "No
	// NSApplication, no tray" at the top of this file.
	nsApp   uintptr
	memmove func(dst unsafe.Pointer, src, n uintptr) uintptr

	mainQueue     uintptr
	dispatchAsync func(queue, context, work uintptr)
	callback      uintptr

	targetClass objc.Class

	mu   sync.Mutex
	next uintptr
	work map[uintptr]func()
	// targets maps a GiftTrayTarget to its tray.
	targets map[objc.ID]*macTray

	// scale is the largest backing scale factor of the screens, as the bits
	// of a float64; written on the main thread, read when a state is
	// prepared.
	scale atomic.Uint64
}

var (
	macOnce sync.Once
	mac     *macState
	macErr  error
)

func openPlatform(cfg openConfig) (platform, error) {
	macOnce.Do(func() { mac, macErr = openMac() })
	if macErr != nil {
		return nil, macErr
	}
	// No question about NSApp here: it may not exist yet; see "No
	// NSApplication, no tray — yet" at the top of this file.
	var target objc.ID
	mac.pool(func() {
		target = objc.ID(mac.targetClass).Send(mac.sel.alloc).Send(mac.sel.initSel)
	})
	if target == 0 {
		return nil, fmt.Errorf("%w: the target of the clicks could not be created", ErrUnsupported)
	}
	t := &macTray{s: mac, clicked: cfg.clicked, log: cfg.log, target: target}
	mac.mu.Lock()
	mac.targets[target] = t
	mac.mu.Unlock()
	return t, nil
}

func openMac() (*macState, error) {
	var appKit uintptr
	for _, fw := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
	} {
		h, err := purego.Dlopen(fw, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("%w: loading %s failed: %v", ErrUnsupported, fw, err)
		}
		appKit = h
	}
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("%w: loading libSystem failed: %v", ErrUnsupported, err)
	}

	s := &macState{work: map[uintptr]func(){}, targets: map[objc.ID]*macTray{}}
	s.scale.Store(math.Float64bits(2))
	if s.nsApp, err = purego.Dlsym(appKit, "NSApp"); err != nil {
		return nil, fmt.Errorf("%w: AppKit has no NSApp: %v", ErrUnsupported, err)
	}
	// dispatch_get_main_queue() is a macro for the address of this symbol.
	if s.mainQueue, err = purego.Dlsym(libSystem, "_dispatch_main_q"); err != nil {
		return nil, fmt.Errorf("%w: libdispatch has no main queue: %v", ErrUnsupported, err)
	}
	purego.RegisterLibFunc(&s.memmove, libSystem, "memmove")
	purego.RegisterLibFunc(&s.dispatchAsync, libSystem, "dispatch_async_f")

	r := objc.RegisterName
	s.sel = macSel{
		poolClass:             objc.GetClass("NSAutoreleasePool"),
		stringClass:           objc.GetClass("NSString"),
		dataClass:             objc.GetClass("NSData"),
		imageClass:            objc.GetClass("NSImage"),
		statusBarClass:        objc.GetClass("NSStatusBar"),
		menuClass:             objc.GetClass("NSMenu"),
		menuItemClass:         objc.GetClass("NSMenuItem"),
		screenClass:           objc.GetClass("NSScreen"),
		objectClass:           objc.GetClass("NSObject"),
		alloc:                 r("alloc"),
		initSel:               r("init"),
		retain:                r("retain"),
		release:               r("release"),
		drain:                 r("drain"),
		initWithBytes:         r("initWithBytes:length:encoding:"),
		dataWithBytes:         r("dataWithBytes:length:"),
		initWithData:          r("initWithData:"),
		setSize:               r("setSize:"),
		setTemplate:           r("setTemplate:"),
		systemStatusBar:       r("systemStatusBar"),
		statusItemWithLength:  r("statusItemWithLength:"),
		removeStatusItem:      r("removeStatusItem:"),
		button:                r("button"),
		setImage:              r("setImage:"),
		setToolTip:            r("setToolTip:"),
		setTitle:              r("setTitle:"),
		setMenu:               r("setMenu:"),
		menu:                  r("menu"),
		initWithTitle:         r("initWithTitle:"),
		initItem:              r("initWithTitle:action:keyEquivalent:"),
		setAutoenables:        r("setAutoenablesItems:"),
		addItem:               r("addItem:"),
		separatorItem:         r("separatorItem"),
		setTarget:             r("setTarget:"),
		setTag:                r("setTag:"),
		tag:                   r("tag"),
		setEnabled:            r("setEnabled:"),
		setState:              r("setState:"),
		setSubmenu:            r("setSubmenu:"),
		trayClick:             r("trayClick:"),
		screens:               r("screens"),
		count:                 r("count"),
		objectAtIndex:         r("objectAtIndex:"),
		backingScale:          r("backingScaleFactor"),
		setAccessibilityLabel: r("setAccessibilityLabel:"),
	}
	if s.sel.statusBarClass == 0 || s.sel.menuClass == 0 || s.sel.menuItemClass == 0 || s.sel.imageClass == 0 {
		return nil, fmt.Errorf("%w: AppKit loaded but its classes are not there", ErrUnsupported)
	}

	s.targetClass, err = objc.RegisterClass("GiftTrayTarget", s.sel.objectClass, nil, nil, []objc.MethodDef{{
		Cmd: s.sel.trayClick,
		Fn: func(self objc.ID, _ objc.SEL, sender objc.ID) {
			s.click(self, objc.Send[int](sender, s.sel.tag))
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	// The one callback of the package; see the top of this file. It is made
	// last, so that a failure above does not spend one.
	s.callback = purego.NewCallback(func(h uintptr) { s.run(h) })
	return s, nil
}

// hasApp reports whether NSApp has been set, without creating it.
func (s *macState) hasApp() bool {
	var app uintptr
	s.memmove(unsafe.Pointer(&app), s.nsApp, unsafe.Sizeof(app))
	return app != 0
}

// async runs f on the main thread, later, inside an autorelease pool. It
// never waits.
func (s *macState) async(f func()) {
	s.mu.Lock()
	s.next++
	h := s.next
	s.work[h] = f
	s.mu.Unlock()
	s.dispatchAsync(s.mainQueue, h, s.callback)
}

// run is the callback of the main queue: it runs the function stored under h.
func (s *macState) run(h uintptr) {
	s.mu.Lock()
	f := s.work[h]
	delete(s.work, h)
	s.mu.Unlock()
	if f == nil {
		return
	}
	// On the main thread, inside a callback: the goroutine cannot leave
	// this thread before the callback returns, so the pool is drained where
	// it was pushed without a LockOSThread of its own.
	p := objc.ID(s.sel.poolClass).Send(s.sel.alloc).Send(s.sel.initSel)
	defer p.Send(s.sel.drain)
	f()
}

// pool runs f inside an NSAutoreleasePool on one locked OS thread, for the
// one call made off the main thread: creating a click target, which is a
// plain NSObject. The lock is a correctness requirement, for the reason
// clipboard_darwin.go gives at length: the pool is pushed onto the calling
// thread's stack and must be drained from the same thread.
func (s *macState) pool(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := objc.ID(s.sel.poolClass).Send(s.sel.alloc).Send(s.sel.initSel)
	defer p.Send(s.sel.drain)
	f()
}

// click is the action of every clickable entry, on the main thread.
func (s *macState) click(target objc.ID, tag int) {
	s.mu.Lock()
	t := s.targets[target]
	s.mu.Unlock()
	low, id := splitTag(tag)
	if t == nil || id == 0 {
		return
	}
	gen := t.gens.resolve(low)
	go t.clicked(gen, id)
}

// readScale reads the largest backing scale factor of the screens and
// remembers it for the next state that is prepared. Main thread only.
func (s *macState) readScale() float64 {
	best := 0.0
	screens := objc.ID(s.sel.screenClass).Send(s.sel.screens)
	n := objc.Send[uint](screens, s.sel.count)
	for i := range n {
		best = max(best, objc.Send[float64](screens.Send(s.sel.objectAtIndex, i), s.sel.backingScale))
	}
	if best < 1 || best > 4 {
		best = 2 // no screen, or a value no Mac has had
	}
	s.scale.Store(math.Float64bits(best))
	return best
}

// newString builds an owned NSString; the caller releases it.
func (s *macState) newString(str string) objc.ID {
	b := []byte(str)
	n := len(b)
	if n == 0 {
		b = []byte{0}
	}
	return objc.ID(s.sel.stringClass).Send(s.sel.alloc).Send(s.sel.initWithBytes, unsafe.Pointer(&b[0]), n, nsUTF8StringEncoding)
}

// newImage builds an owned template NSImage of pts by pts points from a PNG,
// or 0 for none. The caller releases it.
func (s *macState) newImage(pic []byte, pts float64) objc.ID {
	if len(pic) == 0 {
		return 0
	}
	data := objc.ID(s.sel.dataClass).Send(s.sel.dataWithBytes, unsafe.Pointer(&pic[0]), len(pic))
	im := objc.ID(s.sel.imageClass).Send(s.sel.alloc).Send(s.sel.initWithData, data)
	if im == 0 {
		return 0
	}
	im.Send(s.sel.setSize, nsSize{pts, pts})
	im.Send(s.sel.setTemplate, true)
	return im
}

// macTray is one [Tray]'s status item.
type macTray struct {
	s       *macState
	clicked func(gen uint64, id uint32)
	log     *slog.Logger
	target  objc.ID

	// gens turns the generation in a tag into the one to report.
	gens menuGens

	want pending

	// last is the latest state prepared by update, whose menu the next
	// update reuses when it carries the same entries. Guarded by lastMu.
	lastMu sync.Mutex
	last   *prepared

	// Main thread only: item is the status item while it is in the bar,
	// retained; shownMenu and shownScale say which menu it has, so that an
	// apply that changes only icon or tooltip leaves an open menu alone.
	item       objc.ID
	shownMenu  uint64
	shownScale float64
	noAppOnce  sync.Once
}

// prepared is a state as far as Go can take it: text, tags, and pictures as
// PNGs at the pixel size of scale. menuGen is the generation of the Set its
// menu comes from, which the tags carry.
type prepared struct {
	st      state
	menuGen uint64
	scale   float64
	icon    []byte
	tooltip string
	menu    []macEntry
}

// macEntry is an [entry] with its picture and the tag of its menu item.
type macEntry struct {
	tag       int
	title     string
	picture   []byte
	disabled  bool
	checked   bool
	separator bool
	children  []macEntry
}

func (t *macTray) update(st state) error {
	menuGen := t.gens.note(st)
	scale := math.Float64frombits(t.s.scale.Load())
	t.lastMu.Lock()
	var p *prepared
	if l := t.last; l != nil && l.menuGen == menuGen && l.scale == scale {
		// SetIcon or SetTooltip: the menu is the one prepared before.
		p = prepareBar(st, scale)
		p.menuGen, p.menu = menuGen, l.menu
	} else {
		p = prepare(st, menuGen, scale)
	}
	t.last = p
	t.lastMu.Unlock()
	if t.want.set(p) {
		t.s.async(t.apply)
	}
	if !t.s.hasApp() {
		return fmt.Errorf("%w: this process has no NSApplication yet, so there is no menu bar to put an icon "+
			"into; the icon appears once the application runs", ErrUnsupported)
	}
	return nil
}

func (t *macTray) remove() error {
	if t.want.set(nil) {
		t.s.async(t.apply)
	}
	return nil
}

// apply makes the bar show what is wanted now. Main thread only.
func (t *macTray) apply() {
	s := t.s
	if !s.hasApp() {
		// A run loop on the main thread without NSApplication; see "No
		// NSApplication, no tray — yet". The state stays wanted.
		t.want.unqueue()
		t.noAppOnce.Do(func() {
			t.logf(slog.LevelWarn, "the main thread runs, but there is no NSApplication; the tray icon waits for one")
		})
		return
	}
	p, changed := t.want.take()
	if !changed {
		return
	}
	bar := objc.ID(s.sel.statusBarClass).Send(s.sel.systemStatusBar)
	if p == nil {
		if t.item != 0 {
			bar.Send(s.sel.removeStatusItem, t.item)
			t.item.Send(s.sel.release)
			t.item = 0
			t.shownMenu = 0
		}
		return
	}
	if sc := s.readScale(); sc != p.scale {
		p = prepare(p.st, p.menuGen, sc)
	}
	if t.item == 0 {
		t.shownMenu = 0
		// The bar keeps the item it returns, autoreleased for us; the
		// retain is ours, until the removal.
		t.item = bar.Send(s.sel.statusItemWithLength, float64(nsVariableStatusItemLength)).Send(s.sel.retain)
		if t.item == 0 {
			t.logf(slog.LevelError, "the status bar refused a new item")
			return
		}
	}
	btn := t.item.Send(s.sel.button)
	if im := s.newImage(p.icon, barIconPoints); im != 0 {
		btn.Send(s.sel.setImage, im)
		im.Send(s.sel.release)
		// The dot of an earlier icon that drew nothing, if there was one.
		none := s.newString("")
		btn.Send(s.sel.setTitle, none)
		none.Send(s.sel.release)
	} else {
		// A symbol that draws nothing at this size would leave an item of
		// no width: there, but impossible to find or to click.
		t.logf(slog.LevelWarn, "the tray icon draws nothing, showing a dot instead")
		btn.Send(s.sel.setImage, objc.ID(0))
		dot := s.newString("•")
		btn.Send(s.sel.setTitle, dot)
		dot.Send(s.sel.release)
	}
	var tip objc.ID
	if p.tooltip != "" {
		tip = s.newString(p.tooltip)
		defer tip.Send(s.sel.release)
	}
	btn.Send(s.sel.setToolTip, tip)
	btn.Send(s.sel.setAccessibilityLabel, tip)

	// The menu of the same Set at the same scale is the one in the item
	// already: SetIcon or SetTooltip, which leave a menu that is open alone.
	if p.menuGen == t.shownMenu && p.scale == t.shownScale {
		return
	}
	// The status item retains the new menu and releases the old one. A menu
	// that is open at this moment is kept by AppKit until it closes, and a
	// click in it carries its own, older, generation.
	m := t.newMenu("", p.menu)
	t.item.Send(s.sel.setMenu, m)
	m.Send(s.sel.release)
	t.shownMenu, t.shownScale = p.menuGen, p.scale
}

// newMenu builds an owned NSMenu of entries. Main thread only.
func (t *macTray) newMenu(title string, entries []macEntry) objc.ID {
	s := t.s
	ns := s.newString(title)
	m := objc.ID(s.sel.menuClass).Send(s.sel.alloc).Send(s.sel.initWithTitle, ns)
	ns.Send(s.sel.release)
	// Without this AppKit enables an item only if its target answers its
	// action, and greys out every entry without OnClick that only says
	// something; Disabled is the package's to decide.
	m.Send(s.sel.setAutoenables, false)
	empty := s.newString("")
	defer empty.Send(s.sel.release)
	for _, e := range entries {
		if e.separator {
			m.Send(s.sel.addItem, objc.ID(s.sel.menuItemClass).Send(s.sel.separatorItem))
			continue
		}
		var action objc.SEL
		if e.tag != 0 {
			action = s.sel.trayClick
		}
		ti := s.newString(e.title)
		it := objc.ID(s.sel.menuItemClass).Send(s.sel.alloc).Send(s.sel.initItem, ti, action, empty)
		ti.Send(s.sel.release)
		if e.tag != 0 {
			it.Send(s.sel.setTarget, t.target)
			it.Send(s.sel.setTag, e.tag)
		}
		it.Send(s.sel.setEnabled, !e.disabled)
		if e.checked {
			it.Send(s.sel.setState, nsControlStateValueOn)
		}
		if im := s.newImage(e.picture, menuIconPoints); im != 0 {
			it.Send(s.sel.setImage, im)
			im.Send(s.sel.release)
		}
		if len(e.children) > 0 {
			sub := t.newMenu(e.title, e.children)
			it.Send(s.sel.setSubmenu, sub)
			sub.Send(s.sel.release)
		}
		m.Send(s.sel.addItem, it)
		it.Send(s.sel.release)
	}
	return m
}

func (t *macTray) logf(level slog.Level, msg string, args ...any) {
	if t.log == nil {
		return
	}
	t.log.Log(context.Background(), level, msg, args...)
}

// pending is the wanted state of one icon and whether a function that
// applies it is on the main queue already. See "The main thread, and only
// through the main queue" at the top of this file.
type pending struct {
	mu     sync.Mutex
	want   *prepared // nil: not in the bar
	dirty  bool
	queued bool
}

// set makes p the wanted state, nil for none, and reports whether a function
// has to be queued to apply it: not while one is queued and has not run.
func (q *pending) set(p *prepared) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.want, q.dirty = p, true
	if q.queued {
		return false
	}
	q.queued = true
	return true
}

// unqueue is the queued function's side when it cannot apply anything yet:
// the state stays wanted, and the next set queues again.
func (q *pending) unqueue() {
	q.mu.Lock()
	q.queued = false
	q.mu.Unlock()
}

// take is the queued function's side: the wanted state, and whether it was
// set since the last take. A set after take queues again.
func (q *pending) take() (*prepared, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queued = false
	changed := q.dirty
	q.dirty = false
	return q.want, changed
}

// prepare does the Go part of showing st at scale: the pictures and the
// tags, which carry menuGen.
func prepare(st state, menuGen uint64, scale float64) *prepared {
	p := prepareBar(st, scale)
	p.menuGen = menuGen
	p.menu = prepareEntries(st.menu, menuGen, pixels(menuIconPoints, scale))
	return p
}

// prepareBar is the part of [prepare] for the icon and the tooltip.
func prepareBar(st state, scale float64) *prepared {
	return &prepared{
		st:      st,
		scale:   scale,
		icon:    templatePNG(st.icon.Rasterize(pixels(barIconPoints, scale))),
		tooltip: st.tooltip,
	}
}

func prepareEntries(es []entry, gen uint64, px int) []macEntry {
	if len(es) == 0 {
		return nil
	}
	out := make([]macEntry, len(es))
	for i, e := range es {
		m := macEntry{
			title:     e.title,
			disabled:  e.disabled,
			checked:   e.checked,
			separator: e.separator,
			children:  prepareEntries(e.children, gen, px),
		}
		if e.id != 0 {
			m.tag = makeTag(gen, e.id)
		}
		if !e.separator && !e.symbol.IsZero() {
			m.picture = templatePNG(e.symbol.Rasterize(px))
		}
		out[i] = m
	}
	return out
}

// pixels is the size in device pixels of pts points at scale.
func pixels(pts, scale float64) int {
	return max(1, int(math.Ceil(pts*scale-1e-9)))
}

// templatePNG encodes a coverage mask as the PNG of a template image: black,
// with the coverage as alpha. It returns nil for no mask.
func templatePNG(m *image.Alpha) []byte {
	if m == nil {
		return nil
	}
	w, h := m.Rect.Dx(), m.Rect.Dy()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.Pix[y*im.Stride+x*4+3] = m.Pix[y*m.Stride+x]
		}
	}
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, im); err != nil {
		return nil
	}
	return b.Bytes()
}

// genBits is how many bits of the generation a tag carries.
const genBits = 31

// makeTag is the tag of the menu item of entry id in the state gen.
func makeTag(gen uint64, id uint32) int {
	return int(gen&(1<<genBits-1))<<32 | int(id)
}

// splitTag is the inverse of [makeTag]: the low bits of the generation and
// the id.
func splitTag(tag int) (low uint64, id uint32) {
	return uint64(tag>>32) & (1<<genBits - 1), uint32(tag)
}

// fullGen is the generation whose low genBits bits are low, at or before
// latest and less than 2^genBits before it. Only a click in a menu that was
// replaced more than two billion times while it stayed open would be taken
// for a newer one.
func fullGen(latest, low uint64) uint64 {
	const span = 1 << genBits
	g := latest&^(span-1) | low
	if g > latest && g >= span {
		g -= span
	}
	return g
}

// menuGens follows which generations carry the same menu; see "Clicks" at
// the top of this file.
type menuGens struct {
	mu sync.Mutex
	// latest is the generation of the latest state handed to update.
	latest uint64
	// since is the generation of the Set whose menu latest carries: every
	// state from since to latest has that menu.
	since uint64
	menu  []entry
}

// note records st, handed to update, and returns the generation of the Set
// its menu comes from.
func (g *menuGens) note(st state) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.since == 0 || !sameMenu(g.menu, st.menu) {
		g.since = st.gen
	}
	g.latest, g.menu = st.gen, st.menu
	return g.since
}

// resolve is the generation to report for a click whose tag carries low:
// the latest one if the menu it was made in is the menu of the latest
// state, its own otherwise, which the portable half then drops.
func (g *menuGens) resolve(low uint64) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	gen := fullGen(g.latest, low)
	if g.since != 0 && gen >= g.since {
		return g.latest
	}
	return gen
}
