//go:build windows

package notify

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is the Windows half: toast notifications of the Windows Runtime,
// reached without cgo through syscall and vtables written by hand.
//
//	ToastNotificationManager.GetDefault()            IToastNotificationManagerStatics5
//	  .CreateToastNotifierWithId(AppID), .History    IToastNotificationManagerForUser
//	XmlDocument.LoadXml(toast)                       IXmlDocumentIO
//	ToastNotification(doc), .Tag, .Group             IToastNotificationFactory, IToastNotification2
//	notifier.Show(toast), .Setting                   IToastNotifier
//	history.RemoveGroupedTagWithId(tag, group, id)   IToastNotificationHistory
//
// The interface ids and the order of the vtables are those of the Windows SDK
// metadata, checked on 01.11.2026 against the generated bindings of
// microsoft/windows-rs. A vtable of the Windows Runtime starts with the six
// methods of IInspectable; the first method of an interface is slot 6.
//
// # One thread
//
// Every call into the Windows Runtime runs on one goroutine locked to one OS
// thread, which joined the multithreaded apartment with RoInitialize. Objects
// of the MTA may be used from any thread of it, but a goroutine is not a
// thread, and one thread that is known to be initialised is simpler to reason
// about than the implicit MTA. The worker is in [winWorker].
//
// # Clicks
//
// A click is delivered through COM: the registry entry of the AppUserModelID
// names a class id as CustomActivator (registry_windows.go), and this process
// registers a class object for that id with CoRegisterClassObject. Windows
// creates an INotificationActivationCallback from it and calls Activate with
// the toast's launch argument, which is the toast's tag. That path covers a
// click on the banner and one in the notification centre, as long as the
// process runs; the in-process Activated event of a toast would only cover
// the banner. No LocalServer32 is registered, so a click after the process
// has ended does nothing, as the package documentation promises.
//
// The two COM objects this process implements are static: one class factory,
// one callback, with vtables of syscall callbacks made once. Their reference
// counts are fixed at one because they live as long as the process.
//
// # Tags
//
// A toast is identified by a tag within a group. Every toast of this package
// is in the group "gift"; a [Notification.ID] gets a tag of its own the first
// time it is shown and keeps it, so that a second Show replaces the toast
// under the same tag, which Windows does natively. A nonce from the start of
// the process keeps the tags of two runs apart.

var (
	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procRoActivateInstance     = combase.NewProc("RoActivateInstance")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")
	procCoRegisterClassObject  = combase.NewProc("CoRegisterClassObject")

	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procGetWindow           = user32.NewProc("GetWindow")
)

// mustGUID parses a braced GUID literal of this file. It is parsed here and
// not by windows.GUIDFromString, which would load ole32 while the package
// initialises, in every program that imports it.
func mustGUID(s string) windows.GUID {
	b, err := hex.DecodeString(strings.NewReplacer("{", "", "}", "", "-", "").Replace(s))
	if err != nil || len(b) != 16 {
		panic("gift/notify: bad GUID literal " + s)
	}
	return guidOf([16]byte(b))
}

var (
	iidIUnknown                        = mustGUID("{00000000-0000-0000-C000-000000000046}")
	iidIClassFactory                   = mustGUID("{00000001-0000-0000-C000-000000000046}")
	iidINotificationActivationCallback = mustGUID("{53E31837-6600-4A81-9395-75CFFE746F94}")
	iidToastManagerStatics5            = mustGUID("{D6F5F569-D40D-407C-8989-88CAB42CFD14}")
	iidToastNotificationFactory        = mustGUID("{04124B20-82C6-4229-B109-FD9ED4662B53}")
	iidToastNotification2              = mustGUID("{9DFB9FD1-143A-490E-90BF-B9FBA7132DE7}")
	iidXmlDocumentIO                   = mustGUID("{6CD0E74E-EE65-4489-9EBF-CA43E87BA637}")
	iidXmlDocument                     = mustGUID("{F7F3A506-1E87-42D6-BCFB-B8C809FA5494}")
)

// Vtable slots. IUnknown is 0 to 2, IInspectable adds 3 to 5.
const (
	slotQueryInterface = 0
	slotRelease        = 2

	slotGetDefault                = 6 // IToastNotificationManagerStatics5
	slotCreateToastNotifierWithId = 7 // IToastNotificationManagerForUser
	slotGetHistory                = 8 // IToastNotificationManagerForUser
	slotShow                      = 6 // IToastNotifier
	slotGetSetting                = 8 // IToastNotifier
	slotCreateToastNotification   = 6 // IToastNotificationFactory
	slotLoadXml                   = 6 // IXmlDocumentIO
	slotPutTag                    = 6 // IToastNotification2
	slotPutGroup                  = 8 // IToastNotification2
	slotRemoveGroupedTagWithId    = 8 // IToastNotificationHistory
)

const (
	roInitMultithreaded = 1
	clsctxLocalServer   = 4
	regclsMultipleUse   = 1

	eNoInterface        = 0x80004002
	classENoAggregation = 0x80040110
	hresultNotFound     = 0x80070490 // HRESULT_FROM_WIN32(ERROR_NOT_FOUND)

	toastGroup = "gift"

	swRestore = 9
	gwOwner   = 4
)

// notificationSetting names the values of Windows.UI.Notifications.NotificationSetting.
var notificationSetting = map[int32]string{
	1: "the user turned notifications off for this application",
	2: "the user turned notifications off",
	3: "a group policy turns notifications off",
}

// vcall calls method slot of the COM object obj with obj as the first
// argument. uintptrescapes keeps pointers that callers convert to uintptr in
// the argument list alive and on the heap for the duration of the call, as
// it does for the call of a syscall.
//
//go:uintptrescapes
func vcall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func failed(r uintptr, what string) error {
	if int32(r) < 0 {
		return fmt.Errorf("gift/notify: %s failed with HRESULT 0x%08X", what, uint32(r))
	}
	return nil
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		vcall(obj, slotRelease)
	}
}

func queryInterface(obj unsafe.Pointer, iid *windows.GUID, what string) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	if err := failed(vcall(obj, slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))), what); err != nil {
		return nil, err
	}
	return out, nil
}

// hstring is an HSTRING, the string of the Windows Runtime: an opaque handle
// that is only ever passed back to Windows.
type hstring uintptr

func newHString(s string) (hstring, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h hstring
	r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h, failed(r, "WindowsCreateString")
}

func (h hstring) free() {
	if h != 0 {
		procWindowsDeleteString.Call(uintptr(h))
	}
}

// withHStrings creates the HSTRINGs of ss, calls fn with them and frees them.
func withHStrings(fn func(hs []hstring) error, ss ...string) error {
	hs := make([]hstring, len(ss))
	defer func() {
		for _, h := range hs {
			h.free()
		}
	}()
	for i, s := range ss {
		h, err := newHString(s)
		if err != nil {
			return err
		}
		hs[i] = h
	}
	return fn(hs)
}

// winWorker runs functions on the one thread that talks to the Windows
// Runtime. See "One thread" at the top of this file.
type winWorker struct {
	calls chan func()
}

func startWorker() (*winWorker, error) {
	w := &winWorker{calls: make(chan func())}
	errc := make(chan error)
	go func() {
		runtime.LockOSThread()
		r, _, _ := procRoInitialize.Call(roInitMultithreaded)
		if err := failed(r, "RoInitialize"); err != nil {
			errc <- err
			return
		}
		errc <- nil
		for f := range w.calls {
			f()
		}
	}()
	return w, <-errc
}

func (w *winWorker) do(f func() error) error {
	errc := make(chan error, 1)
	w.calls <- func() { errc <- f() }
	return <-errc
}

// winApp is the process wide state of one AppUserModelID.
type winApp struct {
	w        *winWorker
	aumid    string
	log      *slog.Logger
	notifier unsafe.Pointer // IToastNotifier
	history  unsafe.Pointer // IToastNotificationHistory
	factory  unsafe.Pointer // IToastNotificationFactory
	nonce    string

	mu     sync.Mutex
	seq    uint64
	tags   map[string]string // Notification.ID → tag
	clicks map[string]func() // tag → click
	order  []string

	settingNoted atomic.Bool // the unreliable setting was logged
}

var (
	winMu   sync.Mutex
	winApps = map[string]*winApp{}
	worker  *winWorker
)

// winPlatform is one [Notifier]'s view of its application's state.
type winPlatform struct {
	app       *winApp
	activated func(func())
}

func openPlatform(cfg openConfig) (platform, error) {
	if err := checkAUMID(cfg.appID); err != nil {
		return nil, err
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		return nil, fmt.Errorf("%w: this process runs as administrator, and Windows shows no toasts for an elevated process", ErrUnsupported)
	}
	winMu.Lock()
	defer winMu.Unlock()
	if app := winApps[cfg.appID]; app != nil {
		return &winPlatform{app: app, activated: cfg.activated}, nil
	}
	if err := register(cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if worker == nil {
		w, err := startWorker()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		worker = w
	}
	app := &winApp{
		w:      worker,
		aumid:  cfg.appID,
		log:    cfg.log,
		nonce:  fmt.Sprintf("%x", time.Now().UnixNano()&0xffffffffff),
		tags:   map[string]string{},
		clicks: map[string]func(){},
	}
	if err := worker.do(app.open); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	winApps[cfg.appID] = app
	return &winPlatform{app: app, activated: cfg.activated}, nil
}

// open creates the notifier, the history and the toast factory, and
// registers the activator's class object. It runs on the worker.
func (a *winApp) open() error {
	var statics unsafe.Pointer
	err := withHStrings(func(hs []hstring) error {
		r, _, _ := procRoGetActivationFactory.Call(uintptr(hs[0]), uintptr(unsafe.Pointer(&iidToastManagerStatics5)), uintptr(unsafe.Pointer(&statics)))
		return failed(r, "RoGetActivationFactory(ToastNotificationManager)")
	}, "Windows.UI.Notifications.ToastNotificationManager")
	if err != nil {
		return err
	}
	defer release(statics)

	var forUser unsafe.Pointer
	if err := failed(vcall(statics, slotGetDefault, uintptr(unsafe.Pointer(&forUser))), "ToastNotificationManager.GetDefault"); err != nil {
		return err
	}
	defer release(forUser)

	err = withHStrings(func(hs []hstring) error {
		return failed(vcall(forUser, slotCreateToastNotifierWithId, uintptr(hs[0]), uintptr(unsafe.Pointer(&a.notifier))),
			"CreateToastNotifierWithId")
	}, a.aumid)
	if err != nil {
		return err
	}
	if err := failed(vcall(forUser, slotGetHistory, uintptr(unsafe.Pointer(&a.history))), "ToastNotificationManagerForUser.History"); err != nil {
		return err
	}
	err = withHStrings(func(hs []hstring) error {
		r, _, _ := procRoGetActivationFactory.Call(uintptr(hs[0]), uintptr(unsafe.Pointer(&iidToastNotificationFactory)), uintptr(unsafe.Pointer(&a.factory)))
		return failed(r, "RoGetActivationFactory(ToastNotification)")
	}, "Windows.UI.Notifications.ToastNotification")
	if err != nil {
		return err
	}

	comObjectsOnce.Do(makeCOMObjects)
	clsid := guidOf(activatorCLSID(a.aumid))
	var cookie uint32
	r, _, _ := procCoRegisterClassObject.Call(uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&classFactory)),
		clsctxLocalServer, regclsMultipleUse, uintptr(unsafe.Pointer(&cookie)))
	if err := failed(r, "CoRegisterClassObject"); err != nil {
		// Toasts still work; only clicks do not arrive.
		a.logf(slog.LevelWarn, "clicks on notifications will not be delivered", "error", err)
	}
	return nil
}

// guidOf turns the bytes of a UUID into a GUID, whose first three fields are
// integers read most significant byte first.
func guidOf(u [16]byte) windows.GUID {
	g := windows.GUID{
		Data1: binary.BigEndian.Uint32(u[0:4]),
		Data2: binary.BigEndian.Uint16(u[4:6]),
		Data3: binary.BigEndian.Uint16(u[6:8]),
	}
	copy(g.Data4[:], u[8:16])
	return g
}

// setting reads IToastNotifier.Setting and turns a switch of the user or of a
// group policy into [ErrNotPermitted]. It runs on the worker.
//
// An application without a package cannot read its setting: on Windows 11
// 26H2 the call fails with ERROR_NOT_FOUND although the toast appears. That
// failure, any other one and DisabledByManifest, which names a manifest the
// application does not have, say nothing about whether Windows drops the
// toast, so Show tries anyway.
func (a *winApp) setting() error {
	var s int32
	if hr := vcall(a.notifier, slotGetSetting, uintptr(unsafe.Pointer(&s))); int32(hr) < 0 {
		if hr != hresultNotFound && a.settingNoted.CompareAndSwap(false, true) {
			a.logf(slog.LevelWarn, "cannot read whether toasts are on, showing anyway", "appID", a.aumid,
				"err", failed(hr, "ToastNotifier.Setting"))
		}
		return nil
	}
	switch s {
	case 0:
		return nil
	case 4:
		if a.settingNoted.CompareAndSwap(false, true) {
			a.logf(slog.LevelWarn, "Windows reads the toast setting as DisabledByManifest, showing anyway", "appID", a.aumid)
		}
		return nil
	}
	reason := notificationSetting[s]
	if reason == "" {
		reason = fmt.Sprintf("NotificationSetting %d", s)
	}
	return fmt.Errorf("%w: %s", ErrNotPermitted, reason)
}

func (p *winPlatform) authorize(done func(bool)) {
	a := p.app
	err := a.w.do(a.setting)
	if err != nil {
		a.logf(slog.LevelWarn, "toasts are off", "appID", a.aumid, "err", err)
	}
	done(err == nil)
}

func (p *winPlatform) show(n Notification) error {
	a := p.app
	a.mu.Lock()
	tag := a.tags[n.ID]
	if tag == "" {
		a.seq++
		tag = fmt.Sprintf("%s-%d", a.nonce, a.seq)
		if n.ID != "" {
			a.tags[n.ID] = tag
		}
	}
	a.mu.Unlock()
	if n.OnClick != nil {
		onClick := n.OnClick
		a.remember(tag, func() { p.activated(onClick) })
	} else {
		a.forget(tag)
	}

	x, cut := toastXML(tag, n.Title, n.Body)
	if cut {
		a.logf(slog.LevelWarn, "notification text is longer than a toast holds, truncating", "id", n.ID)
	}
	err := a.w.do(func() error {
		if err := a.setting(); err != nil {
			return err
		}
		return a.showToast(x, tag)
	})
	if err != nil {
		a.forget(tag)
	}
	return err
}

// showToast builds and shows one toast. It runs on the worker.
func (a *winApp) showToast(x, tag string) error {
	var inspectable unsafe.Pointer
	err := withHStrings(func(hs []hstring) error {
		r, _, _ := procRoActivateInstance.Call(uintptr(hs[0]), uintptr(unsafe.Pointer(&inspectable)))
		return failed(r, "RoActivateInstance(XmlDocument)")
	}, "Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return err
	}
	defer release(inspectable)

	io, err := queryInterface(inspectable, &iidXmlDocumentIO, "QueryInterface(IXmlDocumentIO)")
	if err != nil {
		return err
	}
	defer release(io)
	err = withHStrings(func(hs []hstring) error {
		return failed(vcall(io, slotLoadXml, uintptr(hs[0])), "XmlDocument.LoadXml")
	}, x)
	if err != nil {
		return err
	}
	doc, err := queryInterface(inspectable, &iidXmlDocument, "QueryInterface(IXmlDocument)")
	if err != nil {
		return err
	}
	defer release(doc)

	var toast unsafe.Pointer
	if err := failed(vcall(a.factory, slotCreateToastNotification, uintptr(doc), uintptr(unsafe.Pointer(&toast))), "CreateToastNotification"); err != nil {
		return err
	}
	defer release(toast)

	toast2, err := queryInterface(toast, &iidToastNotification2, "QueryInterface(IToastNotification2)")
	if err != nil {
		return err
	}
	defer release(toast2)
	err = withHStrings(func(hs []hstring) error {
		if err := failed(vcall(toast2, slotPutTag, uintptr(hs[0])), "ToastNotification.Tag"); err != nil {
			return err
		}
		return failed(vcall(toast2, slotPutGroup, uintptr(hs[1])), "ToastNotification.Group")
	}, tag, toastGroup)
	if err != nil {
		return err
	}
	return failed(vcall(a.notifier, slotShow, uintptr(toast)), "ToastNotifier.Show")
}

func (p *winPlatform) withdraw(id string) error {
	a := p.app
	a.mu.Lock()
	tag := a.tags[id]
	delete(a.tags, id)
	a.mu.Unlock()
	if tag == "" {
		return nil
	}
	a.forget(tag)
	return a.w.do(func() error {
		return withHStrings(func(hs []hstring) error {
			return failed(vcall(a.history, slotRemoveGroupedTagWithId, uintptr(hs[0]), uintptr(hs[1]), uintptr(hs[2])),
				"ToastNotificationHistory.RemoveGroupedTagWithId")
		}, tag, toastGroup, a.aumid)
	})
}

func (a *winApp) remember(tag string, fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.clicks[tag]; !ok {
		a.order = append(a.order, tag)
	}
	a.clicks[tag] = fn
	for len(a.order) > maxClicks {
		delete(a.clicks, a.order[0])
		a.order = a.order[1:]
	}
}

func (a *winApp) forget(tag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.clicks[tag]; !ok {
		return
	}
	delete(a.clicks, tag)
	for i, t := range a.order {
		if t == tag {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
}

func (a *winApp) logf(level slog.Level, msg string, args ...any) {
	if a.log != nil {
		a.log.Log(context.Background(), level, msg, args...)
	}
}

// maxClicks bounds the table of click handlers; Windows reports neither the
// dismissal nor the expiry of a toast to an application without the
// Dismissed event, which this package does not subscribe to.
const maxClicks = 256

// activate is the click: Windows calls INotificationActivationCallback
// .Activate on a thread of its own with the AppUserModelID and the toast's
// launch argument.
func activate(aumid, launch string) {
	winMu.Lock()
	a := winApps[aumid]
	winMu.Unlock()
	if a == nil {
		return
	}
	a.mu.Lock()
	fn := a.clicks[launch]
	a.mu.Unlock()
	if fn == nil {
		return
	}
	raiseWindow()
	fn()
}

// raiseWindow brings the first visible top-level window of this process to
// the front. Windows grants a process that a toast activates the right to
// take the foreground, which is the only reason SetForegroundWindow works
// here.
func raiseWindow() {
	pid := uint32(os.Getpid())
	raiseMu.Lock()
	defer raiseMu.Unlock()
	raisePID, raiseFound = pid, 0
	_ = windows.EnumWindows(enumWindowsCallback, nil)
	h := raiseFound
	if h == 0 {
		return
	}
	if r, _, _ := procIsIconic.Call(uintptr(h)); r != 0 {
		procShowWindow.Call(uintptr(h), swRestore)
	}
	procSetForegroundWindow.Call(uintptr(h))
}

var (
	raiseMu    sync.Mutex
	raisePID   uint32
	raiseFound windows.HWND

	enumWindowsCallback = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(h, &pid); err != nil || pid != raisePID {
			return 1
		}
		if !windows.IsWindowVisible(h) {
			return 1
		}
		if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner != 0 {
			return 1
		}
		raiseFound = h
		return 0
	})
)

// The two COM objects this process implements. See "Clicks" at the top of
// this file. They are package variables, so that the garbage collector never
// moves or frees what Windows holds pointers to.
type comObject struct {
	vtbl *[5]uintptr
}

var (
	comObjectsOnce sync.Once
	classFactory   comObject
	activator      comObject
	factoryVtbl    [5]uintptr
	activatorVtbl  [5]uintptr
)

func makeCOMObjects() {
	addRef := windows.NewCallback(func(this unsafe.Pointer) uintptr { return 1 })
	queryInterface := func(iids ...windows.GUID) uintptr {
		return windows.NewCallback(func(this unsafe.Pointer, riid *windows.GUID, ppv *unsafe.Pointer) uintptr {
			for _, iid := range iids {
				if *riid == iid {
					*ppv = this
					return 0
				}
			}
			*ppv = nil
			return eNoInterface
		})
	}

	factoryVtbl = [5]uintptr{
		queryInterface(iidIUnknown, iidIClassFactory),
		addRef,
		addRef, // Release
		// CreateInstance
		windows.NewCallback(func(this, outer unsafe.Pointer, riid *windows.GUID, ppv *unsafe.Pointer) uintptr {
			*ppv = nil
			if outer != nil {
				return classENoAggregation
			}
			if *riid != iidIUnknown && *riid != iidINotificationActivationCallback {
				return eNoInterface
			}
			*ppv = unsafe.Pointer(&activator)
			return 0
		}),
		// LockServer
		windows.NewCallback(func(this unsafe.Pointer, lock uintptr) uintptr { return 0 }),
	}
	classFactory.vtbl = &factoryVtbl

	activatorVtbl = [5]uintptr{
		queryInterface(iidIUnknown, iidINotificationActivationCallback),
		addRef,
		addRef, // Release
		// Activate(appUserModelId, invokedArgs, data, count)
		windows.NewCallback(func(this unsafe.Pointer, aumid, args *uint16, data unsafe.Pointer, count uint32) uintptr {
			activate(windows.UTF16PtrToString(aumid), windows.UTF16PtrToString(args))
			return 0
		}),
	}
	activator.vtbl = &activatorVtbl
}
