package filedialog

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/worldiety/gift/internal/dbus"
)

// This file is the GNOME and KDE half of the package, as far as it is
// protocol and not socket: the FileChooser of xdg-desktop-portal, which
// GNOME (xdg-desktop-portal-gnome, -gtk) and KDE (xdg-desktop-portal-kde)
// both implement and draw with their own toolkit. Like gift/notify's
// freedesktop.go and gift/tray's sni.go it carries **no build tag on
// purpose**: it sits on [portalBus] rather than on a connection, and is
// tested on the Mac this project is developed on against a bus that lives in
// the test. platform_linux.go only dials the session bus and hands the
// connection over.
//
// # The calls
//
//	Properties.Get("org.freedesktop.portal.FileChooser", "version") → v   once, when the platform opens
//	GetNameOwner("org.freedesktop.portal.Desktop") → s                    once, and after the portal restarted
//	OpenFile(s parent_window, s title, a{sv} options) → o handle          per Open, OpenMulti, Directory
//	SaveFile(s parent_window, s title, a{sv} options) → o handle          per Save
//	org.freedesktop.portal.Request.Close()                                per close from code
//	signal org.freedesktop.portal.Request.Response(u code, a{sv} results)  the answer
//
// The portal does not answer a call with the paths: it answers with the
// object path of a Request, shows the dialog, and emits Response on that
// path when the person is done. The path is predictable — the caller's
// unique name and a token the caller chose — and the match rule for the
// signal is added before the call, so that a dialog closed at once, or a
// portal that emits before it replies, is not missed. Portals before 0.9
// (2017) did not honour the token; if the handle differs from the
// prediction, a second rule is added for it, and a Response that came
// between the reply and that rule is lost. No desktop this package supports
// still ships such a portal.
//
// # The parent window
//
// parent_window is "x11:<XID>" of the gift window, which makes the dialog
// modal to it on GNOME and KDE, or "" when it is not known; see
// x11parent_linux.go. It is looked up once per dialog, in open, which runs on
// the goroutine of the dialog's request: never on the reader goroutine, and
// never on the UI goroutine, since the lookup may take up to 200 ms.
//
// # Options and versions
//
// The interface has a version property, and options newer than the portal
// are ignored by it rather than refused: directory came with version 3,
// current_folder for OpenFile with version 4, so on an older portal a
// Directory dialog would silently be a file dialog. It is refused here
// instead, for that request only, as a non-permanent [ErrUnsupported], and a
// start folder is left out of OpenFile below version 4. SaveFile has had
// current_folder since version 1.
//
// Filters are globs. The portal matches them case-sensitively on GNOME and
// case-insensitively on KDE; the in-app picker matches case-insensitively,
// and so does a bracket glob, ".jpg" becoming "*.[jJ][pP][gG]", which both
// GTK and Qt understand. MIME type filters would avoid that, but the
// application states extensions, and the mapping from one to the other is a
// database this package does not carry.
//
// # What the reader goroutine may do
//
// Signals arrive on the reader goroutine of the connection, which must not
// block and must not make a call: the reply would be read by the goroutine
// that waits for it. The handler only checks and looks up; the answer is
// handed to a goroutine of the portal that calls done and removes the match
// rules.
//
// # A portal that goes away
//
// If the portal leaves the bus while a dialog is open — it crashed, the
// session's service manager restarted it — no Response will ever come, and
// the one dialog a process may show would be held for good. The portal
// therefore watches NameOwnerChanged for its well-known name: the requests
// of the old owner end with an error, and the next Open asks the bus for the
// new one (which activates it, if need be). A session bus that goes away is
// not detected: the requests it had wait forever, and every later call
// fails, which is a session that is ending.

const (
	portalName       = "org.freedesktop.portal.Desktop"
	portalPath       = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	fileChooserIface = "org.freedesktop.portal.FileChooser"
	requestIface     = "org.freedesktop.portal.Request"
	requestPrefix    = "/org/freedesktop/portal/desktop/request/"
)

// The response codes of org.freedesktop.portal.Request.Response.
const (
	responseSuccess   = 0
	responseCancelled = 1
)

// The filter types of the FileChooser: a glob and a MIME type.
const filterGlob = 0

// portalResponseQueue is how many answers wait for the deliverer before
// the reader hands them over from a goroutine of their own. One dialog is
// open at a time; the queue is never full in practice.
const portalResponseQueue = 8

// portalBus is what the portal needs of a [dbus.Conn], as an interface so
// that a test can play the bus and the portal.
type portalBus interface {
	Call(dest string, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error)
	SetSignalHandler(fn func(*dbus.Message))
	UniqueName() string
}

// portal is the Linux platform: the FileChooser of xdg-desktop-portal.
type portal struct {
	bus       portalBus
	log       *slog.Logger
	responses chan portalAnswer

	mu sync.Mutex
	// owner is the unique name of the portal, the sender its signals carry;
	// "" after it left the bus, until the next open asks again.
	owner   string
	version uint32
	pending map[dbus.ObjectPath]*portalRequest

	// parent returns the parent_window of the next dialog; nil is "". It
	// is set before the portal is used and not changed after.
	parent func() string
}

// portalRequest is one dialog between its call and its Response.
type portalRequest struct {
	done  func(result)
	owner string

	// The fields below are guarded by portal.mu.
	// paths are the paths the Response may come on: the predicted one, and
	// the handle the portal returned, if that differs.
	paths []dbus.ObjectPath
	rules []string
	// handle is the Request object, which Close is called on.
	handle dbus.ObjectPath
	// gone is set when the request left pending: answered, closed or
	// failed. closed is set by close; an answer still on its way to done
	// is then dropped. delivering is set by deliver once it has decided to
	// call done, so that a close after that is not final.
	gone, closed, delivering bool
}

// portalAnswer is a Response, or the end of the portal, on its way from the
// reader goroutine to done.
type portalAnswer struct {
	q     *portalRequest
	rules []string
	body  []any // the Response's code and results; nil with err
	err   error
}

// newPortal subscribes to the comings and goings of the portal, asks for
// the version of its FileChooser and for its unique name. A failure of
// either means that there is no portal, or none with a FileChooser, and is
// a lasting [ErrUnsupported].
//
// The signal handler is installed before any match rule, so that nothing
// that matches is dropped for want of one.
func newPortal(bus portalBus, log *slog.Logger) (*portal, error) {
	p := &portal{
		bus:       bus,
		log:       log,
		responses: make(chan portalAnswer, portalResponseQueue),
		pending:   map[dbus.ObjectPath]*portalRequest{},
	}
	bus.SetSignalHandler(p.signal)
	rule := "type='signal',sender='" + dbus.BusName + "',interface='" + dbus.BusInterface +
		"',member='NameOwnerChanged',path='" + string(dbus.BusPath) + "',arg0='" + portalName + "'"
	if _, err := bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule); err != nil {
		return nil, permanent(fmt.Errorf("%w: subscribing to NameOwnerChanged: %v", ErrUnsupported, err))
	}
	owner, version, err := p.connect()
	if err != nil {
		return nil, permanent(fmt.Errorf("%w: %v", ErrUnsupported, err))
	}
	logf(log, slog.LevelDebug, "xdg-desktop-portal FileChooser found", "version", version, "owner", owner)
	go p.deliver()
	return p, nil
}

// connect asks the portal for the version of its FileChooser, which starts
// it if it is activatable, and then the bus for its unique name.
func (p *portal) connect() (owner string, version uint32, err error) {
	reply, err := p.bus.Call(portalName, portalPath, dbus.PropertiesInterface, "Get", "ss", fileChooserIface, "version")
	if err != nil {
		return "", 0, fmt.Errorf("no xdg-desktop-portal FileChooser on the session bus: %v", err)
	}
	if len(reply) == 1 {
		if v, ok := reply[0].(dbus.Variant); ok {
			version, _ = v.Value.(uint32)
		}
	}
	if version < 1 {
		return "", 0, fmt.Errorf("the xdg-desktop-portal FileChooser reports no version (%v)", reply)
	}
	reply, err = p.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "GetNameOwner", "s", portalName)
	if err != nil {
		return "", 0, fmt.Errorf("asking the bus for the owner of %s: %v", portalName, err)
	}
	if len(reply) == 1 {
		owner, _ = reply[0].(string)
	}
	if owner == "" {
		return "", 0, fmt.Errorf("the bus names no owner of %s", portalName)
	}
	p.mu.Lock()
	p.owner, p.version = owner, version
	p.mu.Unlock()
	return owner, version, nil
}

func (p *portal) open(r request, done func(result)) (func() bool, error) {
	p.mu.Lock()
	owner, version := p.owner, p.version
	p.mu.Unlock()
	if owner == "" {
		var err error
		if owner, version, err = p.connect(); err != nil {
			return nil, fmt.Errorf("gift/filedialog: the portal left the session bus: %w", err)
		}
	}

	parent := ""
	if p.parent != nil {
		parent = p.parent()
	}
	token := newToken()
	opts, err := buildOptions(r, token, version)
	if err != nil {
		return nil, err
	}
	path := requestPath(p.bus.UniqueName(), token)
	rule := responseRule(owner, path)
	q := &portalRequest{done: done, owner: owner, paths: []dbus.ObjectPath{path}, rules: []string{rule}, handle: path}
	p.mu.Lock()
	if p.owner != owner {
		// The portal left between the read above and now; its
		// NameOwnerChanged has ended what was pending and would not
		// end q.
		p.mu.Unlock()
		return nil, errors.New("gift/filedialog: the portal left the session bus while the dialog was being opened")
	}
	p.pending[path] = q
	p.mu.Unlock()

	method := "OpenFile"
	if r.mode == Save {
		method = "SaveFile"
	}
	_, err = p.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule)
	if err != nil {
		err = fmt.Errorf("gift/filedialog: subscribing to the portal's Response: %w", err)
	}
	var reply []any
	if err == nil {
		reply, err = p.bus.Call(portalName, portalPath, fileChooserIface, method, "ssa{sv}", parent, r.title, opts)
		if err != nil {
			err = fmt.Errorf("gift/filedialog: %s of the portal: %w", method, err)
		}
	}
	if err != nil {
		rules, ok, _ := p.forget(q, false)
		if !ok {
			// An answer came before the error — a portal that
			// contradicts itself, or one that left the bus meanwhile —
			// and is on its way to done, which must be the only report.
			return func() bool { return p.close(q) }, nil
		}
		p.removeMatches(rules)
		return nil, err
	}

	var handle dbus.ObjectPath
	if len(reply) == 1 {
		handle, _ = reply[0].(dbus.ObjectPath)
	}
	if handle != "" && handle != path {
		p.track(q, handle)
	}
	return func() bool { return p.close(q) }, nil
}

// track adds the handle the portal returned in place of the predicted path.
func (p *portal) track(q *portalRequest, handle dbus.ObjectPath) {
	logf(p.log, slog.LevelWarn, "the portal ignored handle_token, so its answer may be lost; this is a portal before 0.9",
		"handle", handle)
	rule := responseRule(q.owner, handle)
	p.mu.Lock()
	if q.gone {
		p.mu.Unlock()
		return
	}
	q.paths = append(q.paths, handle)
	q.rules = append(q.rules, rule)
	q.handle = handle
	p.pending[handle] = q
	p.mu.Unlock()
	if _, err := p.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule); err != nil {
		logf(p.log, slog.LevelWarn, "subscribing to the portal's Response", "error", err)
	}
	// The request may have ended between the two locks, and its rules been
	// removed before this one was added; removing it again is harmless.
	p.mu.Lock()
	gone := q.gone
	p.mu.Unlock()
	if gone {
		p.removeMatches([]string{rule})
	}
}

// forget takes q out of pending and returns its match rules, or reports that
// it was gone already. closed marks it as closed from code; delivering
// reports that done is being called for it all the same.
func (p *portal) forget(q *portalRequest, closed bool) (rules []string, ok, delivering bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if closed {
		q.closed = true
	}
	if q.gone {
		return nil, false, q.delivering
	}
	return p.forgetLocked(q), true, false
}

func (p *portal) forgetLocked(q *portalRequest) []string {
	q.gone = true
	for _, path := range q.paths {
		if p.pending[path] == q {
			delete(p.pending, path)
		}
	}
	return append([]string(nil), q.rules...)
}

// close ends q from code. The portal sends no Response for a closed
// request, and an answer already on its way is dropped, so it is final,
// unless deliver had already decided to call done: then done follows.
func (p *portal) close(q *portalRequest) bool {
	p.mu.Lock()
	handle := q.handle
	p.mu.Unlock()
	rules, ok, delivering := p.forget(q, true)
	if !ok {
		return !delivering
	}
	// close is called from the UI goroutine; the round trips are not.
	go func() {
		if _, err := p.bus.Call(portalName, handle, requestIface, "Close", ""); err != nil {
			logf(p.log, slog.LevelDebug, "closing the portal's request", "handle", handle, "error", err)
		}
		p.removeMatches(rules)
	}()
	return true
}

func (p *portal) removeMatches(rules []string) {
	for _, rule := range rules {
		_, _ = p.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "RemoveMatch", "s", rule)
	}
}

// signal handles the signals of the connection. It runs on the reader
// goroutine and must neither block nor call; see the top of this file.
func (p *portal) signal(m *dbus.Message) {
	switch {
	case m.Interface == requestIface && m.Member == "Response":
		if m.Sig != "ua{sv}" {
			return
		}
		p.mu.Lock()
		q := p.pending[m.Path]
		if q == nil || m.Sender == "" || m.Sender != q.owner {
			p.mu.Unlock()
			return
		}
		rules := p.forgetLocked(q)
		p.mu.Unlock()
		p.hand(portalAnswer{q: q, rules: rules, body: m.Body})
	case m.Interface == dbus.BusInterface && m.Member == "NameOwnerChanged":
		if m.Sender != dbus.BusName || m.Path != dbus.BusPath || m.Sig != "sss" || m.Body[0] != portalName {
			return
		}
		old, _ := m.Body[1].(string)
		if old == "" {
			// A portal that arrives changes nothing that waits; open
			// asks for it when it needs one.
			return
		}
		var ended []portalAnswer
		p.mu.Lock()
		if p.owner == old {
			p.owner = ""
		}
		for _, q := range p.pending {
			if q.owner == old && !q.gone {
				ended = append(ended, portalAnswer{q: q, rules: p.forgetLocked(q),
					err: errors.New("the portal left the session bus while the dialog was open")})
			}
		}
		p.mu.Unlock()
		for _, a := range ended {
			p.hand(a)
		}
	}
}

// hand passes a to the deliverer without blocking the reader.
func (p *portal) hand(a portalAnswer) {
	select {
	case p.responses <- a:
	default:
		go func() { p.responses <- a }()
	}
}

// deliver calls done for each answer, unless its request was closed from
// code meanwhile, and removes its match rules.
func (p *portal) deliver() {
	for a := range p.responses {
		res := result{err: a.err}
		if a.err == nil {
			res = parseResponse(a.body[0].(uint32), a.body[1])
		}
		p.mu.Lock()
		closed := a.q.closed
		if !closed {
			a.q.delivering = true
		}
		p.mu.Unlock()
		if !closed {
			a.q.done(res)
		}
		p.removeMatches(a.rules)
	}
}

// formatParent is the parent_window of the X11 window xid: "x11:" and the
// XID in lowercase hexadecimal, as the portal specification writes it, or ""
// for no window.
func formatParent(xid uint64) string {
	if xid == 0 {
		return ""
	}
	return fmt.Sprintf("x11:%x", xid)
}

// responseRule is the match rule for the Response of the request at path,
// sent by owner.
func responseRule(owner string, path dbus.ObjectPath) string {
	return "type='signal',sender='" + owner + "',interface='" + requestIface +
		"',member='Response',path='" + string(path) + "'"
}

// requestPath is the object path the portal gives the request a caller with
// the unique name unique makes with token: the name without its colon and
// with its dots as underscores.
func requestPath(unique, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(unique, ":"), ".", "_")
	return dbus.ObjectPath(requestPrefix + sender + "/" + token)
}

var tokenSerial atomic.Uint64

// newToken returns a handle_token: letters, digits and underscores, as an
// object path element must be, unique within the process by its counter and
// unlikely to meet a token of a previous process with the same unique name
// by its random part.
func newToken() string {
	return fmt.Sprintf("gift%d_%08x", tokenSerial.Add(1), rand.Uint32())
}

// buildOptions turns r into the options of OpenFile or SaveFile for a
// portal of the given version.
func buildOptions(r request, token string, version uint32) ([]dbus.DictEntry, error) {
	opt := func(key string, sig dbus.Signature, v any) dbus.DictEntry {
		return dbus.DictEntry{Key: key, Value: dbus.Variant{Sig: sig, Value: v}}
	}
	opts := []dbus.DictEntry{
		opt("handle_token", "s", token),
		opt("modal", "b", true),
		opt("accept_label", "s", r.accept),
	}
	switch r.mode {
	case OpenMulti:
		opts = append(opts, opt("multiple", "b", true))
	case Directory:
		if version < 3 {
			return nil, fmt.Errorf("%w: the portal's FileChooser is version %d, choosing a folder takes version 3",
				ErrUnsupported, version)
		}
		opts = append(opts, opt("directory", "b", true))
	}
	if len(r.exts) > 0 && r.mode != Directory {
		patterns := make([]any, 0, len(r.exts))
		for _, e := range r.exts {
			patterns = append(patterns, []any{uint32(filterGlob), ciGlob(e)})
		}
		filter := []any{r.filterName, patterns}
		opts = append(opts, opt("filters", "a(sa(us))", []any{filter}), opt("current_filter", "(sa(us))", filter))
	}
	if r.start != "" && (r.mode == Save || version >= 4) {
		opts = append(opts, opt("current_folder", "ay", append([]byte(r.start), 0)))
	}
	if r.mode == Save && r.suggested != "" {
		opts = append(opts, opt("current_name", "s", r.suggested))
	}
	return opts, nil
}

// ciGlob is the glob that matches names ending in ext, whatever their case:
// ".tar.gz" is "*.[tT][aA][rR].[gG][zZ]". The characters a glob gives a
// meaning are bracketed, so that they stand for themselves.
func ciGlob(ext string) string {
	var b strings.Builder
	b.WriteByte('*')
	for _, c := range ext {
		lo, up := unicode.ToLower(c), unicode.ToUpper(c)
		switch {
		case lo != up:
			b.WriteByte('[')
			b.WriteRune(lo)
			b.WriteRune(up)
			b.WriteByte(']')
		case c == '*' || c == '?' || c == '[':
			b.WriteByte('[')
			b.WriteRune(c)
			b.WriteByte(']')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// parseResponse turns the code and the results of a Response into a result.
func parseResponse(code uint32, results any) result {
	switch code {
	case responseSuccess:
	case responseCancelled:
		return result{cancelled: true}
	default:
		return result{err: fmt.Errorf("gift/filedialog: the portal ended the request (code %d)", code)}
	}
	entries, _ := results.([]dbus.DictEntry)
	for _, e := range entries {
		if e.Key != "uris" {
			continue
		}
		v, ok := e.Value.(dbus.Variant)
		if !ok || v.Sig != "as" {
			return result{err: fmt.Errorf("gift/filedialog: the portal answered with uris of type %q", v.Sig)}
		}
		uris, _ := v.Value.([]any)
		paths := make([]string, 0, len(uris))
		for _, u := range uris {
			s, _ := u.(string)
			path, err := fileURI(s)
			if err != nil {
				return result{err: err}
			}
			paths = append(paths, path)
		}
		return result{paths: paths}
	}
	return result{err: errors.New("gift/filedialog: the portal answered success without uris")}
}

// fileURI is the path of a file:// URI on this machine.
func fileURI(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("gift/filedialog: the portal answered with %q: %v", s, err)
	}
	switch {
	case u.Scheme != "file":
		return "", fmt.Errorf("gift/filedialog: the portal answered with %q, not a file URI", s)
	case u.Host != "" && u.Host != "localhost":
		return "", fmt.Errorf("gift/filedialog: the portal answered with %q, a file on another host", s)
	case u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/"):
		return "", fmt.Errorf("gift/filedialog: the portal answered with %q, not an absolute file URI", s)
	case strings.Contains(strings.ToLower(s), "%2f") || strings.ContainsRune(u.Path, 0):
		// An escaped slash or NUL is no part of a file name; GLib refuses
		// both.
		return "", fmt.Errorf("gift/filedialog: the portal answered with %q, an impossible file name", s)
	}
	return u.Path, nil
}

// portalAvailable reports whether the session looks like GNOME or KDE and
// has a session bus, from the environment alone. XDG_CURRENT_DESKTOP is a
// colon-separated list ("ubuntu:GNOME"); without it, the older
// XDG_SESSION_DESKTOP and DESKTOP_SESSION name the session ("plasma",
// "gnome-xorg"). Other desktops may have a portal too, but they are not
// what this package promises, and their FileChooser may be a backend that
// shows nothing.
func portalAvailable(getenv func(string) string, exists func(string) bool) bool {
	names := strings.Split(getenv("XDG_CURRENT_DESKTOP"), ":")
	if getenv("XDG_CURRENT_DESKTOP") == "" {
		names = []string{getenv("XDG_SESSION_DESKTOP"), getenv("DESKTOP_SESSION")}
	}
	desktop := false
	for _, n := range names {
		n = strings.ToLower(n)
		if strings.Contains(n, "gnome") || strings.Contains(n, "kde") || strings.Contains(n, "plasma") {
			desktop = true
		}
	}
	if !desktop {
		return false
	}
	_, err := dbus.SessionBusAddress(getenv, exists)
	return err == nil
}
