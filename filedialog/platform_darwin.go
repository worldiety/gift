//go:build darwin && !ios

package filedialog

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// This file is the macOS half: NSOpenPanel and NSSavePanel, through the
// Objective-C runtime, the calls an AppKit program would make.
//
//	panel := [NSOpenPanel openPanel]                  or [NSSavePanel savePanel]
//	[panel setCanChooseFiles:] [panel setAllowedContentTypes:] ...
//	[panel beginSheetModalForWindow:window completionHandler:^(NSModalResponse r){ ... }]
//	[panel cancel:nil]                                closing from code
//
// # Never a modal loop of its own
//
// AppKit may be touched only on the main thread, and the main thread belongs
// to Ebitengine: it sits in GLFW's event loop and serves the functions the
// game thread hands it. [NSSavePanel runModal] would start a nested modal
// loop on that thread and return only when the panel closes; for all that
// time Ebitengine's own queue on the main thread would wait, and with it
// every Update and Draw of the application. The panel is therefore always
// begun with a completion handler, which returns at once and leaves the main
// thread to its run loop: as a sheet on the key window of the application,
// else its main window, and as a free panel of an activated application when
// there is no window at all, as in a test binary.
//
// Everything that touches AppKit is handed to the main dispatch queue with
// dispatch_async_f, which the run loop of the main thread serves in its
// common modes. dispatch_sync is never used: a caller that is itself on the
// main thread would wait for itself for ever. The helpers that do this —
// hasApp, async, run and pool — are copied from tray/platform_darwin.go,
// whose comment explains them at length; a shared internal package is a
// later clean-up, and tray stays as it is.
//
// # One callback, and blocks
//
// The function that crosses into C for dispatch_async_f is a purego
// callback, a resource the process gets about two thousand of and never gives
// back. There is exactly one, made when the package first opens, and what it
// runs is looked up in a table under the context word: a number, not a Go
// pointer, which C may not hold. The completion handler is an Objective-C
// block from objc.NewBlock, which spends a callback per function type, not
// per block, so every panel shares one. AppKit copies the block when the
// panel begins; the reference made here is released straight after.
//
// # Lifetime of a panel
//
// openPanel and savePanel return an autoreleased panel, which is retained
// here for as long as it is shown. The completion handler marks the request
// finished and hands the release to a later turn of the main queue, so that
// the panel never goes away under AppKit while AppKit is still calling out
// of it. close asks on the main thread, after the flag: a panel that has
// finished, and may be released, is never sent cancel:. The flag is read and
// written on the main thread only, which is what makes it safe without a
// lock. close does not stop the completion handler: it comes with Cancel,
// and the portable half drops it and frees the dialog for the next one
// (final is false).
//
// # No NSApplication, no panel — yet
//
// A panel needs an NSApplication whose run loop runs. A test binary or a
// command line tool has none, and neither has a gift application before
// backend.Run: Ebitengine creates NSApp inside RunGame. So the question is
// asked at every open, not once, by reading AppKit's global NSApp rather
// than calling [NSApplication sharedApplication], which would create the
// application; see "No NSApplication, no tray — yet" in tray. Without one,
// open fails with an [ErrUnsupported] that answers this request only.
//
// # Filters, folders and names
//
// The extensions become UTTypes, through typeWithFilenameExtension:, and
// the panel lets through what conforms to one of them: ".jpg" also admits a
// ".jpeg" file. An extension the system cannot map is skipped; none left
// sets no filter at all rather than an empty one, which would admit nothing.
// The save panel appends the first type to a typed name, and
// allowsOtherFileTypes lets the person keep an extension of their own.
//
// The start directory and the picked paths cross as file system
// representations, the bytes the kernel sees, so a name that is not valid
// UTF-8 survives both ways.
//
// A sandboxed application without the entitlement
// com.apple.security.files.user-selected.read-only (or read-write) cannot
// show a panel at all; AppKit logs the refusal, and whatever it answers
// ends the request as a cancel. That case has not been tried here.

// nsModalResponseOK is NSModalResponseOK, an NSInteger; Cancel is 0.
const nsModalResponseOK = 1

// macSel holds the classes and selectors, resolved once.
type macSel struct {
	poolClass, stringClass, urlClass, arrayClass objc.Class
	openPanelClass, savePanelClass, typeClass    objc.Class

	alloc, initSel, drain, retain, release objc.SEL
	stringWithUTF8String                   objc.SEL
	openPanel, savePanel                   objc.SEL

	setCanChooseFiles, setCanChooseDirectories  objc.SEL
	setAllowsMultipleSelection, setCanCreateDir objc.SEL
	setMessage, setTitle, setPrompt             objc.SEL
	setDirectoryURL, setNameFieldStringValue    objc.SEL
	setAllowedContentTypes, setAllowsOtherTypes objc.SEL
	typeWithFilenameExtension                   objc.SEL
	fileURLWithFileSystemRepresentation         objc.SEL
	arrayWithCapacity, addObject                objc.SEL
	urls, url, fileSystemRepresentation         objc.SEL
	count, objectAtIndex                        objc.SEL

	keyWindow, mainWindow, activateIgnoringOtherApps objc.SEL
	beginSheet, begin, cancel, respondsToSelector    objc.SEL
}

// macState is the process wide part: the frameworks, the one callback and
// the table of work for the main queue.
type macState struct {
	sel macSel

	// nsApp is the address of AppKit's global NSApp; see "No
	// NSApplication, no panel — yet" at the top of this file.
	nsApp   uintptr
	memmove func(dst unsafe.Pointer, src, n uintptr) uintptr

	mainQueue     uintptr
	dispatchAsync func(queue, context, work uintptr)
	callback      uintptr

	mu   sync.Mutex
	next uintptr
	work map[uintptr]func()
}

var (
	macOnce sync.Once
	mac     *macState
	macErr  error
)

// platformAvailable is true: every Mac has the panels. Whether this process
// can show one is asked at every open.
func platformAvailable() bool { return true }

func openPlatform(cfg openConfig) (platform, error) {
	macOnce.Do(func() { mac, macErr = openMac() })
	if macErr != nil {
		return nil, permanent(macErr)
	}
	return &macPlatform{s: mac, log: cfg.log}, nil
}

func openMac() (*macState, error) {
	var appKit uintptr
	for _, fw := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
		"/System/Library/Frameworks/UniformTypeIdentifiers.framework/UniformTypeIdentifiers",
	} {
		h, err := purego.Dlopen(fw, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return nil, fmt.Errorf("%w: loading %s failed: %v", ErrUnsupported, fw, err)
		}
		if strings.HasSuffix(fw, "/AppKit") {
			appKit = h
		}
	}
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("%w: loading libSystem failed: %v", ErrUnsupported, err)
	}

	s := &macState{work: map[uintptr]func(){}}
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
		poolClass:      objc.GetClass("NSAutoreleasePool"),
		stringClass:    objc.GetClass("NSString"),
		urlClass:       objc.GetClass("NSURL"),
		arrayClass:     objc.GetClass("NSMutableArray"),
		openPanelClass: objc.GetClass("NSOpenPanel"),
		savePanelClass: objc.GetClass("NSSavePanel"),
		typeClass:      objc.GetClass("UTType"),

		alloc:                r("alloc"),
		initSel:              r("init"),
		drain:                r("drain"),
		retain:               r("retain"),
		release:              r("release"),
		stringWithUTF8String: r("stringWithUTF8String:"),
		openPanel:            r("openPanel"),
		savePanel:            r("savePanel"),

		setCanChooseFiles:          r("setCanChooseFiles:"),
		setCanChooseDirectories:    r("setCanChooseDirectories:"),
		setAllowsMultipleSelection: r("setAllowsMultipleSelection:"),
		setCanCreateDir:            r("setCanCreateDirectories:"),
		setMessage:                 r("setMessage:"),
		setTitle:                   r("setTitle:"),
		setPrompt:                  r("setPrompt:"),
		setDirectoryURL:            r("setDirectoryURL:"),
		setNameFieldStringValue:    r("setNameFieldStringValue:"),
		setAllowedContentTypes:     r("setAllowedContentTypes:"),
		setAllowsOtherTypes:        r("setAllowsOtherFileTypes:"),
		typeWithFilenameExtension:  r("typeWithFilenameExtension:"),
		fileURLWithFileSystemRepresentation: r(
			"fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:"),
		arrayWithCapacity:        r("arrayWithCapacity:"),
		addObject:                r("addObject:"),
		urls:                     r("URLs"),
		url:                      r("URL"),
		fileSystemRepresentation: r("fileSystemRepresentation"),
		count:                    r("count"),
		objectAtIndex:            r("objectAtIndex:"),

		keyWindow:                 r("keyWindow"),
		mainWindow:                r("mainWindow"),
		activateIgnoringOtherApps: r("activateIgnoringOtherApps:"),
		beginSheet:                r("beginSheetModalForWindow:completionHandler:"),
		begin:                     r("beginWithCompletionHandler:"),
		cancel:                    r("cancel:"),
		respondsToSelector:        r("respondsToSelector:"),
	}
	for name, c := range map[string]objc.Class{
		"NSAutoreleasePool": s.sel.poolClass, "NSString": s.sel.stringClass,
		"NSURL": s.sel.urlClass, "NSMutableArray": s.sel.arrayClass,
		"NSOpenPanel": s.sel.openPanelClass, "NSSavePanel": s.sel.savePanelClass,
		"UTType": s.sel.typeClass,
	} {
		if c == 0 {
			return nil, fmt.Errorf("%w: the frameworks loaded but the class %s is not there", ErrUnsupported, name)
		}
	}
	// The one callback of the package; see the top of this file. It is made
	// last, so that a failure above does not spend one.
	s.callback = purego.NewCallback(func(h uintptr) { s.run(h) })
	return s, nil
}

// app is NSApp, or 0 while there is none; it never creates the application.
func (s *macState) app() objc.ID {
	var app uintptr
	s.memmove(unsafe.Pointer(&app), s.nsApp, unsafe.Sizeof(app))
	return objc.ID(app)
}

// hasApp reports whether NSApp has been set, without creating it.
//
// Copied from tray/platform_darwin.go, as are async, run and pool.
func (s *macState) hasApp() bool { return s.app() != 0 }

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
	s.pool(f)
}

// pool runs f inside an NSAutoreleasePool on one locked OS thread. The
// pool is pushed onto the calling thread's stack and must be drained from
// the same thread; on the main thread inside a callback the lock changes
// nothing, and it makes pool safe anywhere else.
func (s *macState) pool(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := objc.ID(s.sel.poolClass).Send(s.sel.alloc).Send(s.sel.initSel)
	defer p.Send(s.sel.drain)
	f()
}

// str is an autoreleased NSString of a Go string, which must be valid
// UTF-8; the request makes it so. It ends at a NUL, as a C string does.
func (s *macState) str(v string) objc.ID {
	if i := strings.IndexByte(v, 0); i >= 0 {
		v = v[:i]
	}
	return objc.ID(s.sel.stringClass).Send(s.sel.stringWithUTF8String, v)
}

// macPlatform is the platform of a process on macOS.
type macPlatform struct {
	s *macState
	// log is the logger of the dialog that opened the platform, for what
	// is only worth a Debug line.
	log *slog.Logger
	// shown, if not nil, is called on the main thread with the panel once
	// it has begun. Only the tests set it.
	shown func(panel objc.ID)
}

// macRequest is one panel. Its fields are touched on the main thread only.
type macRequest struct {
	panel    objc.ID // retained while shown
	finished bool    // the completion handler has run, or the panel never began
}

func (p *macPlatform) open(r request, done func(result)) (func() bool, error) {
	s := p.s
	if !s.hasApp() {
		return nil, fmt.Errorf("%w: this process has no NSApplication yet, so there is nothing to show a panel "+
			"in; a gift application has one once backend.Run has started", ErrUnsupported)
	}
	q := &macRequest{}
	s.async(func() { p.begin(q, r, done) })
	return func() bool {
		s.async(func() {
			if !q.finished && q.panel != 0 {
				q.panel.Send(s.sel.cancel, objc.ID(0))
			}
		})
		return false
	}, nil
}

// begin makes the panel r describes and shows it. Main thread only.
func (p *macPlatform) begin(q *macRequest, r request, done func(result)) {
	s := p.s
	save := r.mode == Save
	var panel objc.ID
	if save {
		panel = objc.ID(s.sel.savePanelClass).Send(s.sel.savePanel)
	} else {
		panel = objc.ID(s.sel.openPanelClass).Send(s.sel.openPanel)
	}
	if panel == 0 {
		q.finished = true
		go done(result{err: fmt.Errorf("AppKit made no panel")})
		return
	}
	q.panel = panel.Send(s.sel.retain)

	switch r.mode {
	case Open, OpenMulti:
		panel.Send(s.sel.setCanChooseFiles, true)
		panel.Send(s.sel.setCanChooseDirectories, false)
		panel.Send(s.sel.setAllowsMultipleSelection, r.mode == OpenMulti)
	case Directory:
		panel.Send(s.sel.setCanChooseFiles, false)
		panel.Send(s.sel.setCanChooseDirectories, true)
		panel.Send(s.sel.setAllowsMultipleSelection, false)
		panel.Send(s.sel.setCanCreateDir, true)
	case Save:
		panel.Send(s.sel.setCanCreateDir, true)
		if r.suggested != "" {
			panel.Send(s.sel.setNameFieldStringValue, s.str(r.suggested))
		}
	}
	// macOS has not shown the title of a panel since Big Sur; the message
	// above the files is where it is read.
	panel.Send(s.sel.setTitle, s.str(r.title))
	panel.Send(s.sel.setMessage, s.str(r.title))
	panel.Send(s.sel.setPrompt, s.str(r.accept))
	if r.start != "" {
		dir := objc.ID(s.sel.urlClass).Send(s.sel.fileURLWithFileSystemRepresentation, r.start, true, objc.ID(0))
		if dir != 0 {
			panel.Send(s.sel.setDirectoryURL, dir)
		}
	}
	if r.mode != Directory {
		if types := p.contentTypes(r.exts); types != 0 {
			panel.Send(s.sel.setAllowedContentTypes, types)
			if save {
				panel.Send(s.sel.setAllowsOtherTypes, true)
			}
		}
	}

	block := objc.NewBlock(func(_ objc.Block, resp int) {
		if q.finished {
			return
		}
		var res result
		s.pool(func() { res = p.answer(q.panel, save, resp) })
		q.finished = true
		// AppKit may still be on its way out of this panel; the release
		// waits for the next turn of the main queue.
		panel := q.panel
		s.async(func() { panel.Send(s.sel.release) })
		go done(res)
	})
	if win := p.window(); win != 0 {
		p.debug("showing the panel as a sheet on the application's window")
		panel.Send(s.sel.beginSheet, win, block)
	} else {
		p.debug("showing a free panel, the application has no window")
		// Deprecated since macOS 14 but still the one call that brings a
		// process without a window to the front; asked first, so that a
		// later macOS without it gets a panel behind other windows rather
		// than an exception.
		if app := s.app(); objc.Send[bool](app, s.sel.respondsToSelector, s.sel.activateIgnoringOtherApps) {
			app.Send(s.sel.activateIgnoringOtherApps, true)
		}
		panel.Send(s.sel.begin, block)
	}
	block.Release()
	if p.shown != nil {
		p.shown(panel)
	}
}

// debug logs at Debug from a goroutine of its own: it is called on the main
// thread, which a slow handler of the application must not hold up.
func (p *macPlatform) debug(msg string, args ...any) {
	if p.log != nil {
		go logf(p.log, slog.LevelDebug, msg, args...)
	}
}

// window is the window a sheet hangs on: the key window of the
// application, else its main window, else 0. Main thread only.
func (p *macPlatform) window() objc.ID {
	app := p.s.app()
	if w := app.Send(p.s.sel.keyWindow); w != 0 {
		return w
	}
	return app.Send(p.s.sel.mainWindow)
}

// contentTypes is an autoreleased NSArray of the UTTypes of exts, or 0 when
// none of them maps to one. Main thread only.
func (p *macPlatform) contentTypes(exts []string) objc.ID {
	if len(exts) == 0 {
		return 0
	}
	s := p.s
	arr := objc.ID(s.sel.arrayClass).Send(s.sel.arrayWithCapacity, uint(len(exts)))
	n := 0
	for _, e := range exts {
		ext := strings.TrimPrefix(e, ".")
		t := objc.ID(s.sel.typeClass).Send(s.sel.typeWithFilenameExtension, s.str(ext))
		if t == 0 {
			p.debug("macOS has no type for an extension, it is left out of the filter", "extension", e)
			continue
		}
		arr.Send(s.sel.addObject, t)
		n++
	}
	if n == 0 {
		return 0
	}
	return arr
}

// answer reads what the panel was closed with. Main thread only.
func (p *macPlatform) answer(panel objc.ID, save bool, resp int) result {
	if resp != nsModalResponseOK {
		return result{cancelled: true}
	}
	s := p.s
	var urls []objc.ID
	if save {
		if u := panel.Send(s.sel.url); u != 0 {
			urls = append(urls, u)
		}
	} else {
		arr := panel.Send(s.sel.urls)
		n := objc.Send[uint](arr, s.sel.count)
		for i := range n {
			urls = append(urls, arr.Send(s.sel.objectAtIndex, i))
		}
	}
	paths := make([]string, 0, len(urls))
	for _, u := range urls {
		path := objc.Send[string](u, s.sel.fileSystemRepresentation)
		if path == "" {
			return result{err: fmt.Errorf("the panel answered with a URL that is not a file")}
		}
		paths = append(paths, path)
	}
	// No URL at all is for the portable half to report.
	return result{paths: paths}
}
