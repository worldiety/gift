//go:build darwin

package notify

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// This file is the macOS half: UNUserNotificationCenter through the
// Objective-C runtime, the calls an AppKit program would make.
//
//	[UNUserNotificationCenter currentNotificationCenter]
//	[center setDelegate:]                                    clicks, and banners while in front
//	[center getNotificationSettingsWithCompletionHandler:]   the permission, before each Show
//	[center requestAuthorizationWithOptions:completionHandler:]
//	[center addNotificationRequest:withCompletionHandler:]
//	[center removeDeliveredNotificationsWithIdentifiers:]
//
// # The bundle check comes first
//
// currentNotificationCenter does not fail for a process that is not an app
// bundle: it raises an exception that nothing catches and the process ends
// with SIGABRT. A program run by `go run` or `go test` is such a process. So
// the mainBundle is asked first, for an identifier and for a path that ends in
// .app, and a process without both never gets to the call. That turns the
// abort into [ErrUnsupported], and it is the reason TestNotABundleIsUnsupported
// runs on every `go test` on a Mac.
//
// A bundle that passes the check can still be refused — unsigned, not
// registered with LaunchServices, or under /var/folders. The system says so
// as UNErrorDomain code 1, which arrives as [ErrNotPermitted].
//
// # Completion handlers
//
// Every call that answers does so through a block, on a queue of the system.
// The blocks this file passes are purego blocks around Go closures that send
// on a channel; the caller waits on it for [callTimeout], the same budget as
// a round trip over D-Bus.
//
// The delegate is called with blocks of the system's, which purego's
// Block.Invoke cannot call: it only knows the blocks it made. A block is a
// struct whose third field is the function to call with the block itself as
// the first argument (the Block ABI of clang), and [callBlock] does exactly
// that.
//
// # Every Show is a new request
//
// The obvious way to replace a notification is to add a request under the
// same identifier, and the system documents exactly that. It loses both
// notifications when the second arrives while the first is still in
// usernoted's pipeline, which takes a few hundred milliseconds: the second
// request removes "similar" ones by identifier, the removal cancels the work
// queued under that identifier, and that is the second request's own work as
// well. The log says CancellationError and nothing is shown. Measured on
// macOS 27.2 on 01.11.2026 with two Shows two milliseconds apart.
//
// So every Show adds a request with an identifier of its own, and a Show
// with an ID that is already shown removes the previous request after the
// new one was accepted. The ID of the application maps to the request in
// macState.current.
//
// # One centre per process
//
// The centre, the delegate and the table of click handlers exist once. Every
// [Notifier] gets its own [macPlatform] in front of them, which only adds its
// own way of posting a click.

// macSel holds the classes and selectors, resolved once.
type macSel struct {
	bundleClass, stringClass, arrayClass, poolClass objc.Class
	centerClass, contentClass, requestClass         objc.Class

	alloc, initSel, release, drain                     objc.SEL
	mainBundle, bundleIdentifier, bundlePath, utf8     objc.SEL
	initWithBytes, arrayWithObject                     objc.SEL
	currentCenter, setDelegate, getSettings, authorize objc.SEL
	authorizationStatus                                objc.SEL
	setTitle, setBody                                  objc.SEL
	requestWith, addRequest, removeDelivered           objc.SEL
	removePending                                      objc.SEL
	actionIdentifier, notification, request            objc.SEL
	identifier, domain, code, localizedDescription     objc.SEL
}

// macState is the process wide part.
type macState struct {
	sel    macSel
	center objc.ID

	// nonce makes request identifiers unique across runs, so that a new run
	// does not replace what an earlier run left in the notification centre.
	nonce string

	mu  sync.Mutex
	seq uint64
	// current maps a [Notification.ID] to the request that shows it now.
	current map[string]string
	clicks  map[string]func()
	order   []string
	asking  bool
	waiters []func(bool)
}

// maxClicks bounds the table of click handlers. macOS does not report that a
// notification was dismissed, so nothing else would ever remove an entry of
// a notification without an ID.
const maxClicks = 256

// The values of UNAuthorizationStatus, UNAuthorizationOptions and
// UNNotificationPresentationOptions, from UNUserNotificationCenter.h. Numbers
// in an ABI.
const (
	unStatusNotDetermined = 0
	unStatusDenied        = 1

	unAuthorizationSound = 1 << 1
	unAuthorizationAlert = 1 << 2

	unPresentSound  = 1 << 1
	unPresentList   = 1 << 3
	unPresentBanner = 1 << 4

	unErrorNotificationsNotAllowed = 1

	nsUTF8StringEncoding = 4
)

// unDefaultAction is UNNotificationDefaultActionIdentifier: the click on the
// notification itself, as opposed to a button or the dismissal.
const unDefaultAction = "com.apple.UNNotificationDefaultActionIdentifier"

var (
	macOnce sync.Once
	mac     *macState
	macErr  error
)

// macPlatform is one [Notifier]'s view of the process wide centre.
type macPlatform struct {
	s         *macState
	activated func(func())
}

func openPlatform(cfg openConfig) (platform, error) {
	macOnce.Do(func() { mac, macErr = openMac() })
	if macErr != nil {
		return nil, macErr
	}
	return &macPlatform{s: mac, activated: cfg.activated}, nil
}

func openMac() (*macState, error) {
	for _, fw := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/UserNotifications.framework/UserNotifications",
	} {
		if _, err := purego.Dlopen(fw, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
			return nil, fmt.Errorf("%w: loading %s failed: %v", ErrUnsupported, fw, err)
		}
	}
	r := objc.RegisterName
	s := &macState{
		sel: macSel{
			bundleClass:          objc.GetClass("NSBundle"),
			stringClass:          objc.GetClass("NSString"),
			arrayClass:           objc.GetClass("NSArray"),
			poolClass:            objc.GetClass("NSAutoreleasePool"),
			centerClass:          objc.GetClass("UNUserNotificationCenter"),
			contentClass:         objc.GetClass("UNMutableNotificationContent"),
			requestClass:         objc.GetClass("UNNotificationRequest"),
			alloc:                r("alloc"),
			initSel:              r("init"),
			release:              r("release"),
			drain:                r("drain"),
			mainBundle:           r("mainBundle"),
			bundleIdentifier:     r("bundleIdentifier"),
			bundlePath:           r("bundlePath"),
			utf8:                 r("UTF8String"),
			initWithBytes:        r("initWithBytes:length:encoding:"),
			arrayWithObject:      r("arrayWithObject:"),
			currentCenter:        r("currentNotificationCenter"),
			setDelegate:          r("setDelegate:"),
			getSettings:          r("getNotificationSettingsWithCompletionHandler:"),
			authorize:            r("requestAuthorizationWithOptions:completionHandler:"),
			authorizationStatus:  r("authorizationStatus"),
			setTitle:             r("setTitle:"),
			setBody:              r("setBody:"),
			requestWith:          r("requestWithIdentifier:content:trigger:"),
			addRequest:           r("addNotificationRequest:withCompletionHandler:"),
			removeDelivered:      r("removeDeliveredNotificationsWithIdentifiers:"),
			removePending:        r("removePendingNotificationRequestsWithIdentifiers:"),
			actionIdentifier:     r("actionIdentifier"),
			notification:         r("notification"),
			request:              r("request"),
			identifier:           r("identifier"),
			domain:               r("domain"),
			code:                 r("code"),
			localizedDescription: r("localizedDescription"),
		},
		nonce:   fmt.Sprintf("%x", time.Now().UnixNano()),
		current: map[string]string{},
		clicks:  map[string]func(){},
	}
	if s.sel.centerClass == 0 || s.sel.contentClass == 0 || s.sel.requestClass == 0 || s.sel.bundleClass == 0 {
		return nil, fmt.Errorf("%w: UserNotifications loaded but its classes are not there", ErrUnsupported)
	}

	var err error
	s.pool(func() {
		b := objc.ID(s.sel.bundleClass).Send(s.sel.mainBundle)
		id := s.goString(b.Send(s.sel.bundleIdentifier))
		path := s.goString(b.Send(s.sel.bundlePath))
		if id == "" || !strings.HasSuffix(path, ".app") {
			err = fmt.Errorf("%w: this program is not an app bundle (%s); macOS shows notifications only for one, "+
				"run it with github.com/worldiety/gift/cmd/giftbundle", ErrUnsupported, path)
			return
		}
		delegate, derr := newDelegate(s)
		if derr != nil {
			err = fmt.Errorf("%w: %v", ErrUnsupported, derr)
			return
		}
		s.center = objc.ID(s.sel.centerClass).Send(s.sel.currentCenter)
		if s.center == 0 {
			err = fmt.Errorf("%w: there is no notification centre", ErrUnsupported)
			return
		}
		// The centre holds its delegate weakly; this one is never released.
		s.center.Send(s.sel.setDelegate, delegate)
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// newDelegate registers the delegate class and returns an instance of it.
func newDelegate(s *macState) (objc.ID, error) {
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("UNUserNotificationCenterDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	class, err := objc.RegisterClass("GiftNotifyDelegate", objc.GetClass("NSObject"), protocols, nil, []objc.MethodDef{
		{
			Cmd: objc.RegisterName("userNotificationCenter:willPresentNotification:withCompletionHandler:"),
			Fn: func(_ objc.ID, _ objc.SEL, _, _ objc.ID, handler unsafe.Pointer) {
				// Without this a notification is not shown at all while
				// the application is the active one, which is the opposite
				// of what Linux and Windows do.
				callBlock(handler, unPresentBanner|unPresentList|unPresentSound)
			},
		},
		{
			Cmd: objc.RegisterName("userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:"),
			Fn: func(_ objc.ID, _ objc.SEL, _, response objc.ID, handler unsafe.Pointer) {
				s.response(response)
				callBlock(handler)
			},
		},
	})
	if err != nil {
		return 0, err
	}
	return objc.ID(class).Send(s.sel.alloc).Send(s.sel.initSel), nil
}

// blockLiteral is the head of a block in the Block ABI of clang: the class,
// two words of flags, and the function that runs it.
type blockLiteral struct {
	isa      uintptr
	flags    uint32
	reserved uint32
	invoke   uintptr
}

// callBlock calls a block the system handed to the delegate. See "Completion
// handlers" at the top of this file.
func callBlock(block unsafe.Pointer, args ...uintptr) {
	if block == nil {
		return
	}
	fn := (*blockLiteral)(block).invoke
	purego.SyscallN(fn, append([]uintptr{uintptr(block)}, args...)...)
}

// response handles a click. It runs on a queue of the system; the click
// handler it finds posts, it does not run OnClick here.
func (s *macState) response(r objc.ID) {
	if s.goString(r.Send(s.sel.actionIdentifier)) != unDefaultAction {
		return
	}
	id := s.goString(r.Send(s.sel.notification).Send(s.sel.request).Send(s.sel.identifier))
	s.mu.Lock()
	fn := s.clicks[id]
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// pool runs f inside an NSAutoreleasePool on one locked OS thread. The lock is
// a correctness requirement, for the reason clipboard_darwin.go gives at
// length: the pool is pushed onto the calling thread's stack and must be
// drained from the same thread.
func (s *macState) pool(f func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := objc.ID(s.sel.poolClass).Send(s.sel.alloc).Send(s.sel.initSel)
	defer p.Send(s.sel.drain)
	f()
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

// goString copies an NSString into Go memory, at most 64 KiB of it: nothing
// this file reads — identifiers, paths, error descriptions — is longer.
func (s *macState) goString(ns objc.ID) string {
	if ns == 0 {
		return ""
	}
	p := objc.Send[unsafe.Pointer](ns, s.sel.utf8)
	if p == nil {
		return ""
	}
	n := 0
	for n < 64<<10 && *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// nsError turns an NSError into a Go error; nil for none. The one the system
// sends for a missing permission wraps [ErrNotPermitted].
func (s *macState) nsError(e objc.ID) error {
	if e == 0 {
		return nil
	}
	domain := s.goString(e.Send(s.sel.domain))
	code := objc.Send[int](e, s.sel.code)
	desc := s.goString(e.Send(s.sel.localizedDescription))
	if domain == "UNErrorDomain" && code == unErrorNotificationsNotAllowed {
		return fmt.Errorf("%w: %s", ErrNotPermitted, desc)
	}
	return fmt.Errorf("gift/notify: %s (%s %d)", desc, domain, code)
}

// status asks the centre for the permission as it is now. It is asked before
// every Show rather than remembered, because the user changes it in the
// system settings without telling the application.
func (s *macState) status() (int, error) {
	ch := make(chan int, 1)
	s.pool(func() {
		block := objc.NewBlock(func(_ objc.Block, settings objc.ID) {
			ch <- objc.Send[int](settings, s.sel.authorizationStatus)
		})
		s.center.Send(s.sel.getSettings, block)
		block.Release()
	})
	select {
	case st := <-ch:
		return st, nil
	case <-time.After(callTimeout):
		return 0, fmt.Errorf("gift/notify: the notification settings did not arrive within %v", callTimeout)
	}
}

// ask shows the system's question, once at a time: a second call while the
// question is open waits for the same answer.
func (s *macState) ask(done func(bool)) {
	s.mu.Lock()
	if done != nil {
		s.waiters = append(s.waiters, done)
	}
	if s.asking {
		s.mu.Unlock()
		return
	}
	s.asking = true
	s.mu.Unlock()

	s.pool(func() {
		block := objc.NewBlock(func(_ objc.Block, granted bool, _ objc.ID) {
			s.mu.Lock()
			w := s.waiters
			s.waiters, s.asking = nil, false
			s.mu.Unlock()
			for _, fn := range w {
				fn(granted)
			}
		})
		s.center.Send(s.sel.authorize, uint(unAuthorizationAlert|unAuthorizationSound), block)
		block.Release()
	})
}

func (p *macPlatform) authorize(done func(bool)) {
	st, err := p.s.status()
	switch {
	case err != nil:
		done(false)
	case st == unStatusNotDetermined:
		p.s.ask(done)
	default:
		done(st != unStatusDenied)
	}
}

func (p *macPlatform) show(n Notification) error {
	s := p.s
	st, err := s.status()
	if err != nil {
		return err
	}
	switch st {
	case unStatusNotDetermined:
		s.ask(nil)
		return fmt.Errorf("%w: the user has not answered yet; the question is shown now", ErrNotPermitted)
	case unStatusDenied:
		return fmt.Errorf("%w: the user turned notifications off for this application", ErrNotPermitted)
	}

	s.mu.Lock()
	s.seq++
	req := fmt.Sprintf("gift-%s-%d", s.nonce, s.seq)
	s.mu.Unlock()
	// The handler is in the table before the request goes out, so that no
	// click can arrive before it.
	if n.OnClick != nil {
		onClick := n.OnClick
		s.remember(req, func() { p.activated(onClick) })
	}

	errc := make(chan error, 1)
	s.pool(func() {
		content := objc.ID(s.sel.contentClass).Send(s.sel.alloc).Send(s.sel.initSel)
		defer content.Send(s.sel.release)
		title, body, ident := s.newString(n.Title), s.newString(n.Body), s.newString(req)
		defer title.Send(s.sel.release)
		defer body.Send(s.sel.release)
		defer ident.Send(s.sel.release)
		content.Send(s.sel.setTitle, title)
		content.Send(s.sel.setBody, body)
		r := objc.ID(s.sel.requestClass).Send(s.sel.requestWith, ident, content, objc.ID(0))
		block := objc.NewBlock(func(_ objc.Block, e objc.ID) {
			errc <- s.nsError(e)
		})
		s.center.Send(s.sel.addRequest, r, block)
		block.Release()
	})
	select {
	case err = <-errc:
	case <-time.After(callTimeout):
		err = fmt.Errorf("gift/notify: the notification centre did not answer within %v", callTimeout)
	}
	if err != nil {
		s.forget(req)
		return err
	}
	if n.ID != "" {
		s.mu.Lock()
		old := s.current[n.ID]
		s.current[n.ID] = req
		s.mu.Unlock()
		if old != "" {
			s.remove(old)
		}
	}
	return nil
}

func (p *macPlatform) withdraw(id string) error {
	s := p.s
	s.mu.Lock()
	req := s.current[id]
	delete(s.current, id)
	s.mu.Unlock()
	if req != "" {
		s.remove(req)
	}
	return nil
}

// remove takes a request out of the notification centre, shown or pending,
// and forgets its click handler.
func (s *macState) remove(req string) {
	s.forget(req)
	s.pool(func() {
		ident := s.newString(req)
		defer ident.Send(s.sel.release)
		ids := objc.ID(s.sel.arrayClass).Send(s.sel.arrayWithObject, ident)
		s.center.Send(s.sel.removeDelivered, ids)
		s.center.Send(s.sel.removePending, ids)
	})
}

func (s *macState) remember(id string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clicks[id]; !ok {
		s.order = append(s.order, id)
	}
	s.clicks[id] = fn
	for len(s.order) > maxClicks {
		delete(s.clicks, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *macState) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clicks[id]; !ok {
		return
	}
	delete(s.clicks, id)
	for i, o := range s.order {
		if o == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func unregisterPlatform(Config) error { return nil }
