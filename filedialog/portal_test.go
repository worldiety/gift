package filedialog

import (
	"bytes"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/worldiety/gift/internal/dbus"
)

// fakePortalBus plays the session bus and the portal on it: it answers the
// version, GetNameOwner, the match rules, OpenFile, SaveFile and Close, and
// lets the test emit signals as the bus would deliver them.
type fakePortalBus struct {
	unique string

	mu      sync.Mutex
	handler func(*dbus.Message)
	calls   []fakeCall
	version uint32
	owner   string
	// hook, if set, answers a call first; handled false falls through to
	// the defaults.
	hook func(c fakeCall) (reply []any, err error, handled bool)

	called chan fakeCall
}

type fakeCall struct {
	dest   string
	path   dbus.ObjectPath
	iface  string
	member string
	args   []any
}

func newFakePortalBus(version uint32) *fakePortalBus {
	return &fakePortalBus{unique: ":1.42", version: version, owner: ":1.7", called: make(chan fakeCall, 1024)}
}

func (b *fakePortalBus) Call(dest string, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error) {
	c := fakeCall{dest, path, iface, member, args}
	b.mu.Lock()
	b.calls = append(b.calls, c)
	hook, version, owner := b.hook, b.version, b.owner
	b.mu.Unlock()
	b.called <- c
	if hook != nil {
		if reply, err, ok := hook(c); ok {
			return reply, err
		}
	}
	switch member {
	case "Get":
		return []any{dbus.Variant{Sig: "u", Value: version}}, nil
	case "GetNameOwner":
		return []any{owner}, nil
	case "AddMatch", "RemoveMatch", "Close":
		return nil, nil
	case "OpenFile", "SaveFile":
		token := optionsOf(c)["handle_token"].Value.(string)
		return []any{requestPath(b.unique, token)}, nil
	}
	return nil, &dbus.Error{Name: dbus.ErrorUnknownMethod}
}

func (b *fakePortalBus) SetSignalHandler(fn func(*dbus.Message)) {
	b.mu.Lock()
	b.handler = fn
	b.mu.Unlock()
}

func (b *fakePortalBus) UniqueName() string { return b.unique }

func (b *fakePortalBus) emit(m *dbus.Message) {
	b.mu.Lock()
	h := b.handler
	b.mu.Unlock()
	h(m)
}

// respond emits a Response on path from the portal.
func (b *fakePortalBus) respond(path dbus.ObjectPath, code uint32, uris ...string) {
	b.mu.Lock()
	owner := b.owner
	b.mu.Unlock()
	b.emit(responseMessage(owner, path, code, uris...))
}

func responseMessage(sender string, path dbus.ObjectPath, code uint32, uris ...string) *dbus.Message {
	list := make([]any, 0, len(uris))
	for _, u := range uris {
		list = append(list, u)
	}
	return &dbus.Message{Type: dbus.TypeSignal, Sender: sender, Path: path, Interface: requestIface,
		Member: "Response", Sig: "ua{sv}",
		Body: []any{code, []dbus.DictEntry{{Key: "uris", Value: dbus.Variant{Sig: "as", Value: list}}}}}
}

// waitCall waits for a call of member and returns it.
func (b *fakePortalBus) waitCall(t *testing.T, member string) fakeCall {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case c := <-b.called:
			if c.member == member {
				return c
			}
		case <-deadline:
			t.Fatalf("no %s call within 5 s", member)
		}
	}
}

func (b *fakePortalBus) members() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, c := range b.calls {
		out = append(out, c.member)
	}
	return out
}

// optionsOf returns the a{sv} of an OpenFile or SaveFile call by key.
func optionsOf(c fakeCall) map[string]dbus.Variant {
	return optionMap(c.args[2].([]dbus.DictEntry))
}

func optionMap(entries []dbus.DictEntry) map[string]dbus.Variant {
	m := map[string]dbus.Variant{}
	for _, e := range entries {
		m[e.Key.(string)] = e.Value.(dbus.Variant)
	}
	return m
}

// results collects what the portal hands to done.
type results chan result

func (r results) done(res result) { r <- res }

func (r results) wait(t *testing.T) result {
	t.Helper()
	select {
	case res := <-r:
		return res
	case <-time.After(5 * time.Second):
		t.Fatal("done was not called within 5 s")
	}
	return result{}
}

func (r results) none(t *testing.T) {
	t.Helper()
	select {
	case res := <-r:
		t.Fatalf("done was called with %+v", res)
	case <-time.After(50 * time.Millisecond):
	}
}

func mustPortal(t *testing.T, b *fakePortalBus) *portal {
	t.Helper()
	p, err := newPortal(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFormatParent(t *testing.T) {
	for xid, want := range map[uint64]string{
		0:          "",
		1:          "x11:1",
		0x4a0000b:  "x11:4a0000b",
		0xABCDEF12: "x11:abcdef12",
		1 << 40:    "x11:10000000000",
	} {
		if got := formatParent(xid); got != want {
			t.Errorf("formatParent(%#x) = %q, want %q", xid, got, want)
		}
	}
}

// TestPortalParentWindow checks that the parent the seam returns is what
// OpenFile and SaveFile get, looked up once per dialog, and that "" stays "".
func TestPortalParentWindow(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	var lookups atomic.Int32
	parent := "x11:4a0000b"
	p.parent = func() string {
		lookups.Add(1)
		return parent
	}
	rs := make(results, 4)
	for _, mode := range []Mode{Open, Save} {
		member := "OpenFile"
		if mode == Save {
			member = "SaveFile"
		}
		before := lookups.Load()
		if _, err := p.open(request{mode: mode, title: "T", accept: "A"}, rs.done); err != nil {
			t.Fatal(err)
		}
		c := b.waitCall(t, member)
		if c.args[0] != parent {
			t.Fatalf("%s parent_window %q, want %q", member, c.args[0], parent)
		}
		if n := lookups.Load() - before; n != 1 {
			t.Fatalf("%s looked the parent up %d times, want once", member, n)
		}
		b.respond(requestPath(b.unique, optionsOf(c)["handle_token"].Value.(string)), 1)
		if res := rs.wait(t); !res.cancelled {
			t.Fatalf("result %+v, want cancelled", res)
		}
	}
	parent = ""
	if _, err := p.open(request{mode: Open, title: "T", accept: "A"}, rs.done); err != nil {
		t.Fatal(err)
	}
	if c := b.waitCall(t, "OpenFile"); c.args[0] != "" {
		t.Fatalf("OpenFile parent_window %q, want \"\"", c.args[0])
	}
}

func TestRequestPath(t *testing.T) {
	if got := requestPath(":1.42", "t"); got != "/org/freedesktop/portal/desktop/request/1_42/t" {
		t.Fatalf("requestPath = %s", got)
	}
}

func TestNewToken(t *testing.T) {
	a, b := newToken(), newToken()
	if a == b {
		t.Fatalf("two tokens %q", a)
	}
	for _, tok := range []string{a, b} {
		if strings.Trim(tok, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") != "" {
			t.Fatalf("token %q has characters an object path element cannot", tok)
		}
	}
}

func TestCIGlob(t *testing.T) {
	for in, want := range map[string]string{
		".jpg":    "*.[jJ][pP][gG]",
		".tar.gz": "*.[tT][aA][rR].[gG][zZ]",
		".7z":     "*.7[zZ]",
		".c++":    "*.[cC]++",
		".a*?[":   "*.[aA][*][?][[]",
		".ü":      "*.[üÜ]",
	} {
		if got := ciGlob(in); got != want {
			t.Errorf("ciGlob(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildOptions(t *testing.T) {
	base := request{title: "T", accept: "Go", filterName: "Images", exts: []string{".jpg", ".png"},
		start: "/home/me", suggested: "a.png"}
	images := []any{"Images", []any{[]any{uint32(0), "*.[jJ][pP][gG]"}, []any{uint32(0), "*.[pP][nN][gG]"}}}
	for _, tc := range []struct {
		name    string
		mode    Mode
		version uint32
		want    []string // the keys beyond handle_token, modal and accept_label
	}{
		{"open v4", Open, 4, []string{"filters", "current_filter", "current_folder"}},
		{"open v3 has no current_folder", Open, 3, []string{"filters", "current_filter"}},
		{"open v2", Open, 2, []string{"filters", "current_filter"}},
		{"multi", OpenMulti, 4, []string{"multiple", "filters", "current_filter", "current_folder"}},
		{"directory v3", Directory, 3, []string{"directory"}},
		{"directory v4", Directory, 4, []string{"directory", "current_folder"}},
		{"save v1", Save, 1, []string{"filters", "current_filter", "current_folder", "current_name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.mode = tc.mode
			entries, err := buildOptions(r, "tok", tc.version)
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, e := range entries {
				keys = append(keys, e.Key.(string))
			}
			want := append([]string{"handle_token", "modal", "accept_label"}, tc.want...)
			if !slices.Equal(keys, want) {
				t.Fatalf("keys %v, want %v", keys, want)
			}
			m := optionMap(entries)
			if m["handle_token"].Value != "tok" || m["modal"].Value != true || m["accept_label"].Value != "Go" {
				t.Fatalf("options %v", m)
			}
			if f, ok := m["filters"]; ok {
				if f.Sig != "a(sa(us))" || !reflect.DeepEqual(f.Value, []any{images}) {
					t.Fatalf("filters %#v", f)
				}
				if c := m["current_filter"]; c.Sig != "(sa(us))" || !reflect.DeepEqual(c.Value, images) {
					t.Fatalf("current_filter %#v", c)
				}
			}
			if f, ok := m["current_folder"]; ok {
				if f.Sig != "ay" || !bytes.Equal(f.Value.([]byte), []byte("/home/me\x00")) {
					t.Fatalf("current_folder %#v, want NUL-terminated bytes", f)
				}
			}
			if n, ok := m["current_name"]; ok && n.Value != "a.png" {
				t.Fatalf("current_name %#v", n)
			}
		})
	}

	r := base
	r.mode = Directory
	_, err := buildOptions(r, "tok", 2)
	var pe permanentError
	if !errors.Is(err, ErrUnsupported) || errors.As(err, &pe) {
		t.Fatalf("Directory on version 2: %v, want an ErrUnsupported that is not permanent", err)
	}

	r = request{mode: Save, title: "Save", accept: "Save", filterName: "x"}
	entries, _ := buildOptions(r, "tok", 4)
	if len(entries) != 3 {
		t.Fatalf("a bare Save sends %v", entries)
	}
}

// TestOptionsSurviveTheWire sends an OpenFile through the encoder and the
// decoder of internal/dbus, so that the Go values above are ones the wire
// format takes for the signature, and come back as a portal reads them.
func TestOptionsSurviveTheWire(t *testing.T) {
	r := request{mode: Save, title: "Save it", accept: "Save", filterName: "Archives",
		exts: []string{".tar.gz"}, start: "/tmp/ü", suggested: "x.tar.gz"}
	opts, err := buildOptions(r, "tok", 4)
	if err != nil {
		t.Fatal(err)
	}
	m := &dbus.Message{Type: dbus.TypeMethodCall, Serial: 1, Path: portalPath, Interface: fileChooserIface,
		Member: "SaveFile", Destination: portalName, Sig: "ssa{sv}", Body: []any{"", r.title, opts}}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := dbus.ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Body[0] != "" || got.Body[1] != "Save it" {
		t.Fatalf("arguments %v", got.Body[:2])
	}
	o := optionMap(got.Body[2].([]dbus.DictEntry))
	filter := []any{"Archives", []any{[]any{uint32(0), "*.[tT][aA][rR].[gG][zZ]"}}}
	if !reflect.DeepEqual(o["filters"], dbus.Variant{Sig: "a(sa(us))", Value: []any{filter}}) {
		t.Fatalf("filters %#v", o["filters"])
	}
	if !reflect.DeepEqual(o["current_filter"], dbus.Variant{Sig: "(sa(us))", Value: filter}) {
		t.Fatalf("current_filter %#v", o["current_filter"])
	}
	if f := o["current_folder"]; f.Sig != "ay" || !bytes.Equal(f.Value.([]byte), []byte("/tmp/ü\x00")) {
		t.Fatalf("current_folder %#v", f)
	}
	if o["current_name"].Value != "x.tar.gz" || o["handle_token"].Value != "tok" || o["modal"].Value != true {
		t.Fatalf("options %v", o)
	}

	// The Response the other way, as the reader hands it to parseResponse.
	resp := responseMessage(":1.7", "/org/freedesktop/portal/desktop/request/1_42/tok", 0,
		"file:///tmp/%C3%BC/x.tar.gz")
	resp.Serial = 2
	b, err = resp.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := dbus.ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	res := parseResponse(back.Body[0].(uint32), back.Body[1])
	if res.err != nil || !slices.Equal(res.paths, []string{"/tmp/ü/x.tar.gz"}) {
		t.Fatalf("parseResponse %+v", res)
	}
}

func TestParseResponse(t *testing.T) {
	uris := func(sig dbus.Signature, v any) []dbus.DictEntry {
		return []dbus.DictEntry{{Key: "choices", Value: dbus.Variant{Sig: "a(ss)", Value: []any{}}},
			{Key: "uris", Value: dbus.Variant{Sig: sig, Value: v}}}
	}
	res := parseResponse(0, uris("as", []any{"file:///a", "file:///b%20c"}))
	if res.err != nil || res.cancelled || !slices.Equal(res.paths, []string{"/a", "/b c"}) {
		t.Fatalf("success: %+v", res)
	}
	if res := parseResponse(1, nil); !res.cancelled || res.err != nil {
		t.Fatalf("code 1: %+v", res)
	}
	if res := parseResponse(2, nil); res.err == nil || !strings.Contains(res.err.Error(), "code 2") {
		t.Fatalf("code 2: %+v", res)
	}
	for name, entries := range map[string][]dbus.DictEntry{
		"no results":   nil,
		"no uris":      {{Key: "choices", Value: dbus.Variant{Sig: "a(ss)", Value: []any{}}}},
		"uris not as":  uris("s", "file:///a"),
		"a bad uri":    uris("as", []any{"file:///a", "https://example.com/b"}),
		"not a string": uris("as", []any{int32(1)}),
	} {
		if res := parseResponse(0, entries); res.err == nil {
			t.Errorf("%s: %+v, want an error", name, res)
		}
	}
	if res := parseResponse(0, uris("as", []any(nil))); res.err != nil || len(res.paths) != 0 {
		t.Fatalf("empty uris: %+v, want no paths, which the core reports", res)
	}
}

func TestFileURI(t *testing.T) {
	for in, want := range map[string]string{
		"file:///home/me/a%20b.txt":   "/home/me/a b.txt",
		"file:///tmp/%C3%BC.txt":      "/tmp/ü.txt",
		"file:///tmp/ü.txt":           "/tmp/ü.txt",
		"file://localhost/x":          "/x",
		"file:///tmp/%FF":             "/tmp/\xff",
		"file:///run/user/1000/doc/a": "/run/user/1000/doc/a",
	} {
		got, err := fileURI(in)
		if err != nil || got != want {
			t.Errorf("fileURI(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"file://host/x", "https://example.com/x", "file:relative", "file:///a%2Fb", "file:///a?b",
		"file:///a#b", "file:///a%00b", "file:///%zz", "", "/plain/path",
	} {
		if got, err := fileURI(in); err == nil {
			t.Errorf("fileURI(%q) = %q, want an error", in, got)
		}
	}
}

func TestPortalAvailable(t *testing.T) {
	bus := map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/run/user/1000/bus"}
	never := func(string) bool { return false }
	for _, tc := range []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{"XDG_CURRENT_DESKTOP": "GNOME"}, true},
		{map[string]string{"XDG_CURRENT_DESKTOP": "ubuntu:GNOME"}, true},
		{map[string]string{"XDG_CURRENT_DESKTOP": "KDE"}, true},
		{map[string]string{"XDG_CURRENT_DESKTOP": "GNOME-Classic:GNOME"}, true},
		{map[string]string{"XDG_SESSION_DESKTOP": "plasma"}, true},
		{map[string]string{"DESKTOP_SESSION": "gnome-xorg"}, true},
		{map[string]string{"DESKTOP_SESSION": "plasmawayland"}, true},
		{map[string]string{"XDG_CURRENT_DESKTOP": "XFCE"}, false},
		{map[string]string{"XDG_CURRENT_DESKTOP": "labwc:wlroots"}, false},
		{map[string]string{"XDG_CURRENT_DESKTOP": "sway"}, false},
		{map[string]string{"XDG_CURRENT_DESKTOP": "XFCE", "DESKTOP_SESSION": "gnome"}, false},
		{map[string]string{}, false},
	} {
		env := func(k string) string {
			if v, ok := tc.env[k]; ok {
				return v
			}
			return bus[k]
		}
		if got := portalAvailable(env, never); got != tc.want {
			t.Errorf("%v: %v, want %v", tc.env, got, tc.want)
		}
	}
	gnome := func(k string) string {
		if k == "XDG_CURRENT_DESKTOP" {
			return "GNOME"
		}
		if k == "XDG_RUNTIME_DIR" {
			return "/run/user/1000"
		}
		return ""
	}
	if portalAvailable(gnome, never) {
		t.Fatal("available without a session bus")
	}
	if !portalAvailable(gnome, func(p string) bool { return p == "/run/user/1000/bus" }) {
		t.Fatal("not available with the bus socket in XDG_RUNTIME_DIR")
	}
}

func TestNewPortalFailuresAreLasting(t *testing.T) {
	for name, hook := range map[string]func(fakeCall) ([]any, error, bool){
		"no portal": func(c fakeCall) ([]any, error, bool) {
			if c.member == "Get" {
				return nil, &dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}, true
			}
			return nil, nil, false
		},
		"no version": func(c fakeCall) ([]any, error, bool) {
			if c.member == "Get" {
				return []any{dbus.Variant{Sig: "u", Value: uint32(0)}}, nil, true
			}
			return nil, nil, false
		},
		"no owner": func(c fakeCall) ([]any, error, bool) {
			if c.member == "GetNameOwner" {
				return nil, &dbus.Error{Name: "org.freedesktop.DBus.Error.NameHasNoOwner"}, true
			}
			return nil, nil, false
		},
		"no AddMatch": func(c fakeCall) ([]any, error, bool) {
			if c.member == "AddMatch" {
				return nil, errors.New("refused"), true
			}
			return nil, nil, false
		},
	} {
		b := newFakePortalBus(4)
		b.hook = hook
		_, err := newPortal(b, nil)
		var pe permanentError
		if !errors.Is(err, ErrUnsupported) || !errors.As(err, &pe) {
			t.Errorf("%s: %v, want a permanent ErrUnsupported", name, err)
		}
	}
}

func TestPortalOpenAndAnswer(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	if m := b.members(); !slices.Equal(m, []string{"AddMatch", "Get", "GetNameOwner"}) {
		t.Fatalf("opening the platform made the calls %v", m)
	}
	rs := make(results, 4)
	if _, err := p.open(request{mode: OpenMulti, title: "Pick", accept: "Open"}, rs.done); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	calls := slices.Clone(b.calls[3:])
	b.mu.Unlock()
	if len(calls) != 2 || calls[0].member != "AddMatch" || calls[1].member != "OpenFile" {
		t.Fatalf("calls %+v, want AddMatch before OpenFile", calls)
	}
	call := calls[1]
	if call.dest != portalName || call.path != portalPath || call.iface != fileChooserIface ||
		call.args[0] != "" || call.args[1] != "Pick" || optionsOf(call)["multiple"].Value != true {
		t.Fatalf("OpenFile %+v", call)
	}
	path := requestPath(b.unique, optionsOf(call)["handle_token"].Value.(string))
	rule := calls[0].args[0].(string)
	for _, part := range []string{"sender=':1.7'", "path='" + string(path) + "'", "member='Response'",
		"interface='org.freedesktop.portal.Request'", "type='signal'"} {
		if !strings.Contains(rule, part) {
			t.Fatalf("rule %q lacks %s", rule, part)
		}
	}

	b.respond(path, 0, "file:///a", "file:///b")
	res := rs.wait(t)
	if !slices.Equal(res.paths, []string{"/a", "/b"}) {
		t.Fatalf("result %+v", res)
	}
	if c := b.waitCall(t, "RemoveMatch"); c.args[0] != rule {
		t.Fatalf("RemoveMatch %v, want %s", c.args, rule)
	}
	// A second Response for the same path is someone else's business.
	b.respond(path, 0, "file:///c")
	rs.none(t)
}

func TestPortalSaveFile(t *testing.T) {
	b := newFakePortalBus(1)
	p := mustPortal(t, b)
	rs := make(results, 4)
	if _, err := p.open(request{mode: Save, title: "Save", accept: "Save", suggested: "n.txt", start: "/s"}, rs.done); err != nil {
		t.Fatal(err)
	}
	c := b.waitCall(t, "SaveFile")
	o := optionsOf(c)
	if o["current_name"].Value != "n.txt" || string(o["current_folder"].Value.([]byte)) != "/s\x00" {
		t.Fatalf("SaveFile options %v", o)
	}
	b.respond(requestPath(b.unique, o["handle_token"].Value.(string)), 1)
	if res := rs.wait(t); !res.cancelled {
		t.Fatalf("result %+v, want cancelled", res)
	}
}

func TestPortalIgnoresOtherSignals(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	rs := make(results, 4)
	p.open(request{mode: Open}, rs.done)
	path := requestPath(b.unique, optionsOf(b.waitCall(t, "OpenFile"))["handle_token"].Value.(string))

	wrongPath := responseMessage(":1.7", path+"x", 0, "file:///a")
	wrongIface := responseMessage(":1.7", path, 0, "file:///a")
	wrongIface.Interface = "org.example.Request"
	wrongSig := &dbus.Message{Type: dbus.TypeSignal, Sender: ":1.7", Path: path, Interface: requestIface,
		Member: "Response", Sig: "u", Body: []any{uint32(0)}}
	wrongSender := responseMessage(":1.99", path, 0, "file:///a")
	noSender := responseMessage("", path, 0, "file:///a")
	wrongMember := responseMessage(":1.7", path, 0, "file:///a")
	wrongMember.Member = "Responded"
	for _, m := range []*dbus.Message{wrongPath, wrongIface, wrongSig, wrongSender, noSender, wrongMember} {
		b.emit(m)
	}
	rs.none(t)
	b.respond(path, 0, "file:///right")
	if res := rs.wait(t); !slices.Equal(res.paths, []string{"/right"}) {
		t.Fatalf("result %+v", res)
	}
}

func TestPortalResponseBeforeTheHandle(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	b.hook = func(c fakeCall) ([]any, error, bool) {
		if c.member != "OpenFile" {
			return nil, nil, false
		}
		path := requestPath(b.unique, optionsOf(c)["handle_token"].Value.(string))
		b.respond(path, 0, "file:///early")
		return []any{path}, nil, true
	}
	rs := make(results, 4)
	if _, err := p.open(request{mode: Open}, rs.done); err != nil {
		t.Fatal(err)
	}
	if res := rs.wait(t); !slices.Equal(res.paths, []string{"/early"}) {
		t.Fatalf("result %+v", res)
	}
	rs.none(t)
}

func TestPortalCloseFromCode(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	rs := make(results, 4)
	closeFn, err := p.open(request{mode: Directory}, rs.done)
	if err != nil {
		t.Fatal(err)
	}
	c := b.waitCall(t, "OpenFile")
	if optionsOf(c)["directory"].Value != true {
		t.Fatalf("options %v", optionsOf(c))
	}
	path := requestPath(b.unique, optionsOf(c)["handle_token"].Value.(string))
	if !closeFn() {
		t.Fatal("close of the portal is not final")
	}
	if cl := b.waitCall(t, "Close"); cl.path != path || cl.iface != requestIface || cl.dest != portalName {
		t.Fatalf("Close %+v, want it on %s", cl, path)
	}
	b.waitCall(t, "RemoveMatch")
	b.respond(path, 1)
	rs.none(t)
	if !closeFn() {
		t.Fatal("a second close is not final")
	}
	p.mu.Lock()
	n := len(p.pending)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d requests still pending", n)
	}
}

func TestPortalCloseDropsAnAnswerOnItsWay(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	rs := make(results, 4)
	closeFn, _ := p.open(request{mode: Open}, rs.done)
	path := requestPath(b.unique, optionsOf(b.waitCall(t, "OpenFile"))["handle_token"].Value.(string))
	// Do what the reader does with a Response, but hand it to the
	// deliverer only after close ran, as if the deliverer were slow.
	p.mu.Lock()
	q := p.pending[path]
	rules := p.forgetLocked(q)
	p.mu.Unlock()
	if !closeFn() {
		t.Fatal("close not final")
	}
	p.hand(portalAnswer{q: q, rules: rules, body: []any{uint32(0), []dbus.DictEntry(nil)}})
	rs.none(t)
}

func TestPortalHandleMismatch(t *testing.T) {
	b := newFakePortalBus(4)
	var logs records
	p, err := newPortal(b, slog.New(&logs))
	if err != nil {
		t.Fatal(err)
	}
	other := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/old/1")
	b.hook = func(c fakeCall) ([]any, error, bool) {
		if c.member == "OpenFile" {
			return []any{other}, nil, true
		}
		return nil, nil, false
	}
	rs := make(results, 4)
	closeFn, err := p.open(request{mode: Open}, rs.done)
	if err != nil {
		t.Fatal(err)
	}
	if logs.count(slog.LevelWarn) != 1 {
		t.Fatal("no warning about the handle")
	}
	b.mu.Lock()
	var rules []string
	for _, c := range b.calls {
		if c.member == "AddMatch" {
			rules = append(rules, c.args[0].(string))
		}
	}
	b.mu.Unlock()
	if len(rules) != 3 || !strings.Contains(rules[2], "path='"+string(other)+"'") {
		t.Fatalf("rules %v, want a second one for %s", rules, other)
	}
	b.respond(other, 0, "file:///x")
	if res := rs.wait(t); !slices.Equal(res.paths, []string{"/x"}) {
		t.Fatalf("result %+v", res)
	}
	removed := map[string]bool{}
	removed[b.waitCall(t, "RemoveMatch").args[0].(string)] = true
	removed[b.waitCall(t, "RemoveMatch").args[0].(string)] = true
	if !removed[rules[1]] || !removed[rules[2]] {
		t.Fatalf("removed %v", removed)
	}

	// Close goes to the handle the portal returned.
	closeFn2, _ := p.open(request{mode: Open}, rs.done)
	closeFn2()
	if c := b.waitCall(t, "Close"); c.path != other {
		t.Fatalf("Close on %s, want %s", c.path, other)
	}
	_ = closeFn
}

func TestPortalCallFailure(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	b.hook = func(c fakeCall) ([]any, error, bool) {
		if c.member == "OpenFile" {
			return nil, &dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}, true
		}
		return nil, nil, false
	}
	rs := make(results, 4)
	_, err := p.open(request{mode: Open}, rs.done)
	var pe permanentError
	if err == nil || errors.Is(err, ErrUnsupported) || errors.As(err, &pe) {
		t.Fatalf("open: %v, want a plain error", err)
	}
	b.waitCall(t, "RemoveMatch")
	p.mu.Lock()
	n := len(p.pending)
	p.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d requests still pending", n)
	}
	rs.none(t)
}

func TestPortalDirectoryNeedsVersion3(t *testing.T) {
	b := newFakePortalBus(2)
	p := mustPortal(t, b)
	rs := make(results, 4)
	if _, err := p.open(request{mode: Directory}, rs.done); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Directory on version 2: %v", err)
	}
	if _, err := p.open(request{mode: Open, start: "/s"}, rs.done); err != nil {
		t.Fatalf("Open on version 2: %v", err)
	}
	if o := optionsOf(b.waitCall(t, "OpenFile")); o["current_folder"].Sig != "" {
		t.Fatalf("OpenFile on version 2 sent current_folder: %v", o)
	}
}

func TestPortalThatLeaves(t *testing.T) {
	b := newFakePortalBus(4)
	p := mustPortal(t, b)
	rs := make(results, 4)
	p.open(request{mode: Open}, rs.done)
	b.waitCall(t, "OpenFile")
	nameOwnerChanged := func(sender, old, new string) *dbus.Message {
		return &dbus.Message{Type: dbus.TypeSignal, Sender: sender, Path: dbus.BusPath, Interface: dbus.BusInterface,
			Member: "NameOwnerChanged", Sig: "sss", Body: []any{portalName, old, new}}
	}
	// Not from the bus, or a portal arriving: nothing ends.
	b.emit(nameOwnerChanged(":1.66", ":1.7", ""))
	b.emit(nameOwnerChanged(dbus.BusName, "", ":1.8"))
	rs.none(t)

	b.emit(nameOwnerChanged(dbus.BusName, ":1.7", ""))
	if res := rs.wait(t); res.err == nil {
		t.Fatalf("result %+v, want an error", res)
	}
	b.waitCall(t, "RemoveMatch")

	// The next dialog finds the new portal.
	b.mu.Lock()
	b.owner = ":1.9"
	b.mu.Unlock()
	if _, err := p.open(request{mode: Open}, rs.done); err != nil {
		t.Fatal(err)
	}
	b.waitCall(t, "Get")
	b.waitCall(t, "GetNameOwner")
	if rule := b.waitCall(t, "AddMatch").args[0].(string); !strings.Contains(rule, "sender=':1.9'") {
		t.Fatalf("rule %s, want the new owner", rule)
	}
	c := b.waitCall(t, "OpenFile")
	b.respond(requestPath(b.unique, optionsOf(c)["handle_token"].Value.(string)), 0, "file:///new")
	if res := rs.wait(t); !slices.Equal(res.paths, []string{"/new"}) {
		t.Fatalf("result %+v", res)
	}
}

// TestPortalThroughTheCore runs dialogs from Open to the callbacks over the
// fake bus.
func TestPortalThroughTheCore(t *testing.T) {
	b := newFakePortalBus(2)
	op := newOpener(func(cfg openConfig) (platform, error) { return newPortal(b, cfg.log) },
		func() bool { return true }, noEnv)
	var o outcome
	mb := newMailbox()

	d := New(o.pick, o.cancel).Mode(OpenMulti)
	d.op = op
	if _, err := d.Open(mb.post); err != nil {
		t.Fatal(err)
	}
	c := b.waitCall(t, "OpenFile")
	b.respond(requestPath(b.unique, optionsOf(c)["handle_token"].Value.(string)), 0, "file:///a", "file:///b")
	mb.run(t)
	if len(o.picked) != 1 || !slices.Equal(o.picked[0], []string{"/a", "/b"}) {
		t.Fatalf("got %+v", o)
	}

	// Directory on version 2 is a cancel, and the portal stays available.
	dir := d.Mode(Directory)
	if _, err := dir.Open(mb.post); err != nil {
		t.Fatal(err)
	}
	mb.run(t)
	if o.cancelled != 1 || !op.available() {
		t.Fatalf("got %+v, available %v", o, op.available())
	}

	// A close from code reports nothing and frees the dialog at once.
	cancel, err := d.Open(mb.post)
	if err != nil {
		t.Fatal(err)
	}
	b.waitCall(t, "OpenFile")
	cancel()
	b.waitCall(t, "Close")
	waitFree(t, op)
	if _, err := d.Open(mb.post); err != nil {
		t.Fatalf("Open after a close from code: %v", err)
	}
	b.waitCall(t, "OpenFile")
	mb.drain()
	if o.cancelled != 1 || len(o.picked) != 1 {
		t.Fatalf("got %+v", o)
	}
}

// waitFree waits until no request holds the dialog of op.
func waitFree(t *testing.T, op *opener) {
	t.Helper()
	for range 500 {
		op.mu.Lock()
		free := op.owner == 0
		op.mu.Unlock()
		if free {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the dialog was not released within 5 s")
}
