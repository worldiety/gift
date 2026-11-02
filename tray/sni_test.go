package tray

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/dbus"
)

// The Linux half, against a bus that lives in the test: fakeSNIBus answers
// the calls of the item to the bus and to the watcher, keeps what the item
// exports, and lets the test call it the way a panel does. Every call and
// every reply goes through the wire format of internal/dbus on the way, so
// that a body the encoder would refuse, or that a host would decode into
// something else, fails here and not on a desktop.
//
// That a real dbus-daemon agrees is what platform_linux_test.go checks, on
// request.

type fakeSNIBus struct {
	mu         sync.Mutex
	watcher    bool
	owned      bool
	calls      []string
	registered []string
	matches    []string
	refuseReg  error
	objects    map[dbus.ObjectPath][]*dbus.Interface
	signals    []*dbus.Message

	// taken are the well-known names someone else owns; denyName, if set,
	// is the answer to every RequestName; emitErr that of every Emit.
	taken    map[string]bool
	denyName error
	emitErr  error

	// onSignal is the handler the item installed; appearOnMatch makes a
	// watcher appear the moment AddMatch is answered, before NameHasOwner.
	onSignal      func(*dbus.Message)
	appearOnMatch bool
}

func (b *fakeSNIBus) SetSignalHandler(fn func(*dbus.Message)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onSignal = fn
}

const fakeUniqueName = ":1.7"

func (b *fakeSNIBus) UniqueName() string { return fakeUniqueName }

func (b *fakeSNIBus) count(member string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, c := range b.calls {
		if c == member {
			n++
		}
	}
	return n
}

func (b *fakeSNIBus) Call(dest string, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, member)
	switch member {
	case "AddMatch":
		b.matches = append(b.matches, args[0].(string))
		if b.appearOnMatch && b.onSignal != nil {
			// The reader goroutine hands the signal on before the reply
			// of the next call; the watcher's name is owned from now on,
			// but NameHasOwner was asked before.
			b.onSignal(nameOwnerChanged(":1.99"))
		}
		return nil, nil
	case "NameHasOwner":
		return []any{b.watcher && args[0] == sniWatcherName}, nil
	case "RequestName":
		if args[1] != uint32(dbus.NameFlagDoNotQueue) {
			return nil, fmt.Errorf("RequestName with flags %v", args[1])
		}
		if b.denyName != nil {
			return nil, b.denyName
		}
		if b.taken[args[0].(string)] {
			return []any{uint32(dbus.RequestNameExists)}, nil
		}
		b.owned = true
		return []any{uint32(dbus.RequestNamePrimaryOwner)}, nil
	case "ReleaseName":
		b.owned = false
		return []any{uint32(1)}, nil
	case "RegisterStatusNotifierItem":
		if dest != sniWatcherName || path != sniWatcherPath || iface != sniWatcherIface {
			return nil, fmt.Errorf("registered at %s %s %s", dest, path, iface)
		}
		if b.refuseReg != nil {
			return nil, b.refuseReg
		}
		if !b.owned && args[0] != fakeUniqueName {
			return nil, errors.New("registered a name the item does not own")
		}
		b.registered = append(b.registered, args[0].(string))
		return nil, nil
	}
	return nil, errors.New("unexpected call " + member)
}

func (b *fakeSNIBus) Emit(path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) error {
	b.mu.Lock()
	emitErr := b.emitErr
	b.mu.Unlock()
	if emitErr != nil {
		return emitErr
	}
	m := &dbus.Message{Type: dbus.TypeSignal, Serial: 1, Path: path, Interface: iface, Member: member, Sig: sig, Body: args}
	raw, err := m.Marshal()
	if err != nil {
		return err
	}
	m, err = dbus.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.signals = append(b.signals, m)
	b.mu.Unlock()
	return nil
}

// Export checks the interfaces the way a real connection does, by
// exporting them on one that is not connected to anything.
func (b *fakeSNIBus) Export(path dbus.ObjectPath, ifaces ...*dbus.Interface) error {
	if err := new(dbus.Conn).Export(path, ifaces...); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.objects == nil {
		b.objects = map[dbus.ObjectPath][]*dbus.Interface{}
	}
	b.objects[path] = ifaces
	return nil
}

func (b *fakeSNIBus) iface(t *testing.T, path dbus.ObjectPath, name string) *dbus.Interface {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, it := range b.objects[path] {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("nothing exports %s at %s", name, path)
	return nil
}

// roundTrip sends body through the wire format as the signature sig and
// returns what a peer decodes.
func roundTrip(t *testing.T, sig dbus.Signature, body []any) []any {
	t.Helper()
	raw, err := (&dbus.Message{Type: dbus.TypeMethodReturn, Serial: 1, ReplySerial: 1, Sig: sig, Body: body}).Marshal()
	if err != nil {
		t.Fatalf("the body (%s) cannot be marshalled: %v", sig, err)
	}
	m, err := dbus.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return m.Body
}

// call calls a method of an exported object as a host would.
func (b *fakeSNIBus) call(t *testing.T, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error) {
	t.Helper()
	m, ok := b.iface(t, path, iface).Methods[member]
	if !ok {
		t.Fatalf("%s has no method %s", iface, member)
	}
	if m.In != sig {
		t.Fatalf("%s.%s takes (%s), the test sends (%s)", iface, member, m.In, sig)
	}
	msg := &dbus.Message{Type: dbus.TypeMethodCall, Path: path, Interface: iface, Member: member, Sig: sig,
		Body: roundTrip(t, sig, args)}
	reply, err := m.Call(msg)
	if err != nil {
		return nil, err
	}
	return roundTrip(t, m.Out, reply), nil
}

// prop reads a property as a host would, through Properties.Get.
func (b *fakeSNIBus) prop(t *testing.T, path dbus.ObjectPath, iface, name string) any {
	t.Helper()
	p, ok := b.iface(t, path, iface).Properties[name]
	if !ok {
		t.Fatalf("%s has no property %s", iface, name)
	}
	return roundTrip(t, "v", []any{dbus.Variant{Sig: p.Sig, Value: p.Get()}})[0].(dbus.Variant).Value
}

func (b *fakeSNIBus) emitted() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, m := range b.signals {
		out = append(out, m.Member)
	}
	b.signals = nil
	return out
}

func (b *fakeSNIBus) registrations() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.registered...)
}

type click struct {
	gen uint64
	id  uint32
}

func newTestSNI(t *testing.T) (*sni, *fakeSNIBus, chan click) {
	t.Helper()
	return newTestSNIOn(t, &fakeSNIBus{watcher: true}, nil)
}

func newTestSNIOn(t *testing.T, b *fakeSNIBus, log *slog.Logger) (*sni, *fakeSNIBus, chan click) {
	t.Helper()
	clicks := make(chan click, 16)
	s, err := newSNI(b, openConfig{appID: "com.example.Photos", log: log,
		clicked: func(gen uint64, id uint32) { clicks <- click{gen, id} }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stop)
	return s, b, clicks
}

// logRecords is a slog.Handler that keeps the messages it is handed.
type logRecords struct {
	mu   sync.Mutex
	msgs []string
}

func (h *logRecords) Enabled(context.Context, slog.Level) bool { return true }
func (h *logRecords) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Level.String()+" "+r.Message)
	return nil
}
func (h *logRecords) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logRecords) WithGroup(string) slog.Handler      { return h }

// count is how many messages at level (such as "WARN") contain part.
func (h *logRecords) count(level, part string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, m := range h.msgs {
		if strings.HasPrefix(m, level+" ") && strings.Contains(m, part) {
			n++
		}
	}
	return n
}

func nameOwnerChanged(owner string) *dbus.Message {
	return &dbus.Message{Type: dbus.TypeSignal, Sender: dbus.BusName, Path: dbus.BusPath, Interface: dbus.BusInterface,
		Member: "NameOwnerChanged", Sig: "sss", Body: []any{sniWatcherName, "", owner}}
}

func show(t *testing.T, s *sni, st state) {
	t.Helper()
	if err := s.update(st); err != nil {
		t.Fatal(err)
	}
	s.flush()
}

// TestAWatcherThatComesLaterGetsTheItem: an application that starts before
// its panel — both at login — gets its icon when the panel claims the
// watcher's name, and until then makes no round trip per update and logs
// one warning that names the GNOME extension.
func TestAWatcherThatComesLaterGetsTheItem(t *testing.T) {
	log := &logRecords{}
	s, b, _ := newTestSNIOn(t, &fakeSNIBus{}, slog.New(log))
	if got := log.count("WARN", "gnome-shell-extension-appindicator"); got != 1 {
		t.Fatalf("%d warnings naming the extension, want 1: %q", got, log.msgs)
	}
	for gen := range uint64(3) {
		show(t, s, state{gen: gen + 1, icon: outline.User, tooltip: fmt.Sprint(gen)})
	}
	if n := b.count("RequestName") + b.count("RegisterStatusNotifierItem"); n != 0 {
		t.Fatalf("%d round trips without a watcher: %v", n, b.calls)
	}
	if got := b.emitted(); len(got) != 0 {
		t.Fatalf("signals %v without a watcher", got)
	}
	b.mu.Lock()
	b.watcher = true
	b.mu.Unlock()
	s.signal(nameOwnerChanged(":1.99"))
	s.flush()
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v after the watcher appeared", got)
	}
	if got := b.prop(t, sniItemPath, sniItemIface, "ToolTip").([]any)[2]; got != "2" {
		t.Fatalf("the watcher reads the tooltip %q, want the last one", got)
	}
}

func TestTheFirstUpdateRegistersTheItem(t *testing.T) {
	s, b, _ := newTestSNI(t)
	if len(b.matches) != 1 || !strings.Contains(b.matches[0], "arg0='org.kde.StatusNotifierWatcher'") ||
		!strings.Contains(b.matches[0], "member='NameOwnerChanged'") {
		t.Fatalf("match rules %q", b.matches)
	}
	if got := b.registrations(); len(got) != 0 {
		t.Fatalf("registered %v before the first update", got)
	}
	show(t, s, state{gen: 1, icon: outline.User, tooltip: "Photos"})
	want := fmt.Sprintf("org.kde.StatusNotifierItem-%d-", os.Getpid())
	got := b.registrations()
	if len(got) != 1 || !strings.HasPrefix(got[0], want) || got[0] != s.name {
		t.Fatalf("registered %v, want %s<n>", got, want)
	}
	// A second update changes properties; it does not register again.
	show(t, s, state{gen: 2, icon: outline.User, tooltip: "Photos 2"})
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registered %d times", len(got))
	}
}

func TestTwoTraysHaveTwoNames(t *testing.T) {
	a, _, _ := newTestSNI(t)
	b, _, _ := newTestSNI(t)
	if a.name == b.name {
		t.Fatalf("both are %s", a.name)
	}
}

func TestTheItemSaysWhatTheSpecificationAsks(t *testing.T) {
	s, b, _ := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, tooltip: "Photos"})
	for name, want := range map[string]any{
		"Category":   "ApplicationStatus",
		"Id":         "com.example.Photos",
		"Title":      "Photos",
		"Status":     "Active",
		"IconName":   "",
		"ItemIsMenu": true,
		"Menu":       dbus.ObjectPath("/MenuBar"),
		"WindowId":   int32(0),
		"ToolTip":    []any{"", []any(nil), "Photos", ""},
	} {
		if got := b.prop(t, sniItemPath, sniItemIface, name); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
	pix := b.prop(t, sniItemPath, sniItemIface, "IconPixmap").([]any)
	if len(pix) != len(sniIconSizes) {
		t.Fatalf("%d pixmaps, want %d", len(pix), len(sniIconSizes))
	}
	for i, p := range pix {
		p := p.([]any)
		w, h, data := p[0].(int32), p[1].(int32), p[2].([]byte)
		if int(w) != sniIconSizes[i] || h != w || len(data) != int(w*h*4) {
			t.Errorf("pixmap %d is %dx%d with %d bytes", i, w, h, len(data))
		}
	}
	// Activate is refused, so that a host which calls it anyway opens the
	// menu instead.
	_, err := b.call(t, sniItemPath, sniItemIface, "Activate", "ii", int32(1), int32(2))
	var be *dbus.Error
	if !errors.As(err, &be) || be.Name != dbus.ErrorUnknownMethod {
		t.Fatalf("Activate: %v", err)
	}
	for _, m := range []string{"SecondaryActivate", "ContextMenu"} {
		if _, err := b.call(t, sniItemPath, sniItemIface, m, "ii", int32(1), int32(2)); err != nil {
			t.Errorf("%s: %v", m, err)
		}
	}
	if _, err := b.call(t, sniItemPath, sniItemIface, "Scroll", "is", int32(1), "vertical"); err != nil {
		t.Errorf("Scroll: %v", err)
	}
}

func TestATrayWithoutAppIDIsNamedAfterTheExecutable(t *testing.T) {
	if got := sniID(""); got == "" || strings.Contains(got, "/") {
		t.Fatalf("Id %q", got)
	}
}

// TestIconPixmapIsARGBInNetworkOrder: the specification's ARGB32 "in network
// byte order" is the byte sequence A, R, G, B, which a host on a little
// endian machine reads as a big endian integer. Getting it wrong shows as a
// blue icon on a red one's place, or as nothing.
func TestIconPixmapIsARGBInNetworkOrder(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	copy(im.Pix, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if got, want := argb32(im), []byte{4, 1, 2, 3, 8, 5, 6, 7}; !bytes.Equal(got, want) {
		t.Fatalf("got % x, want % x", got, want)
	}
}

func TestTheIconIsAWhiteGlyphWithADarkOutline(t *testing.T) {
	im := sniOutlined(outline.User, 32)
	if im == nil || im.Rect.Dx() != 32 {
		t.Fatalf("got %v", im)
	}
	var glyph, rim, corner int
	for y := range 32 {
		for x := range 32 {
			p := im.NRGBAAt(x, y)
			switch {
			case p.A == 0:
			case p.A == 255 && p.R == sniGlyph.R:
				glyph++
			case p.A > 0 && p.A <= sniOutline.A && p.R < 0x20:
				rim++
			}
			if (x == 0 || x == 31) && (y == 0 || y == 31) && p.A != 0 {
				corner++
			}
		}
	}
	if glyph == 0 || rim == 0 || corner != 0 {
		t.Fatalf("%d glyph pixels, %d outline pixels, %d inked corners", glyph, rim, corner)
	}
	// Straight alpha: a half covered edge pixel of the glyph keeps the
	// colour of the glyph rather than being darkened by its alpha.
	plain := tinted(outline.User, 32, sniGlyph)
	for i := 3; i < len(plain.Pix); i += 4 {
		if plain.Pix[i] > 0 && plain.Pix[i] < 255 && plain.Pix[i-3] != sniGlyph.R {
			t.Fatal("tinted premultiplied the colour")
		}
	}
}

// testMenu is a menu with one of everything, and the ids the portable half
// gives its clickable entries: a=1, b=2, c=3, d=4 (disabled), e=5.
func testMenu() []entry {
	return []entry{
		{id: 1, title: "Show_window", symbol: outline.User},
		{separator: true},
		{title: "label only"},
		{title: "Import", children: []entry{
			{id: 2, title: "From camera"},
			{id: 3, title: "From folder", checked: true},
		}},
		{id: 4, title: "Disabled", disabled: true},
		{id: 5, title: "Quit"},
	}
}

// layoutNode is a decoded (ia{sv}av).
type layoutNode struct {
	id       int32
	props    map[string]any
	children []layoutNode
}

func decodeLayout(t *testing.T, v any) layoutNode {
	t.Helper()
	s := v.([]any)
	n := layoutNode{id: s[0].(int32), props: map[string]any{}}
	for _, e := range s[1].([]dbus.DictEntry) {
		n.props[e.Key.(string)] = e.Value.(dbus.Variant).Value
	}
	for _, c := range s[2].([]any) {
		cv := c.(dbus.Variant)
		if cv.Sig != "(ia{sv}av)" {
			t.Fatalf("child of type %s", cv.Sig)
		}
		n.children = append(n.children, decodeLayout(t, cv.Value))
	}
	return n
}

func getLayout(t *testing.T, b *fakeSNIBus, parent, depth int32, names ...string) (uint32, layoutNode) {
	t.Helper()
	if names == nil {
		names = []string{}
	}
	r, err := b.call(t, sniMenuPath, menuIface, "GetLayout", "iias", parent, depth, names)
	if err != nil {
		t.Fatal(err)
	}
	return r[0].(uint32), decodeLayout(t, r[1])
}

func TestGetLayoutDescribesTheMenu(t *testing.T) {
	s, b, _ := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, menu: testMenu()})
	rev, root := getLayout(t, b, 0, -1)
	if rev == 0 {
		t.Fatal("revision 0")
	}
	if root.id != 0 || root.props["children-display"] != "submenu" || len(root.children) != 6 {
		t.Fatalf("root %+v", root)
	}
	ids := map[int32]bool{0: true}
	var walk func(n layoutNode)
	walk = func(n layoutNode) {
		for _, c := range n.children {
			if ids[c.id] {
				t.Fatalf("id %d twice", c.id)
			}
			ids[c.id] = true
			walk(c)
		}
	}
	walk(root)
	if len(ids) != 9 {
		t.Fatalf("%d ids for 9 nodes", len(ids))
	}

	c := root.children
	if c[0].props["label"] != "Show__window" {
		t.Errorf("label %q, want the underscore doubled", c[0].props["label"])
	}
	png0, ok := c[0].props["icon-data"].([]byte)
	if !ok {
		t.Fatalf("no icon-data: %v", c[0].props)
	}
	if cfg, err := png.DecodeConfig(bytes.NewReader(png0)); err != nil || cfg.Width != menuIconPx {
		t.Errorf("icon-data is not a %d pixel PNG: %v %v", menuIconPx, cfg, err)
	}
	if !reflect.DeepEqual(c[1].props, map[string]any{"type": "separator"}) {
		t.Errorf("separator %v", c[1].props)
	}
	if !reflect.DeepEqual(c[2].props, map[string]any{"label": "label only"}) {
		t.Errorf("label %v", c[2].props)
	}
	if c[3].props["children-display"] != "submenu" || len(c[3].children) != 2 {
		t.Errorf("submenu %+v", c[3])
	}
	if got := c[3].children[1].props; got["toggle-type"] != "checkmark" || got["toggle-state"] != int32(1) {
		t.Errorf("checked %v", got)
	}
	if _, ok := c[3].children[0].props["toggle-type"]; ok {
		t.Errorf("an unchecked entry has a toggle: %v", c[3].children[0].props)
	}
	if c[4].props["enabled"] != false {
		t.Errorf("disabled %v", c[4].props)
	}

	// The depth and the property names limit the answer.
	_, flat := getLayout(t, b, 0, 1, "label")
	if len(flat.children) != 6 || len(flat.children[3].children) != 0 {
		t.Fatalf("depth 1 %+v", flat)
	}
	if _, ok := flat.children[0].props["icon-data"]; ok || flat.children[0].props["label"] == nil {
		t.Errorf("filtered properties %v", flat.children[0].props)
	}
	_, none := getLayout(t, b, 0, 0)
	if len(none.children) != 0 {
		t.Errorf("depth 0 has %d children", len(none.children))
	}
	_, sub := getLayout(t, b, c[3].id, -1)
	if sub.id != c[3].id || len(sub.children) != 2 {
		t.Errorf("subtree %+v", sub)
	}
	if _, err := b.call(t, sniMenuPath, menuIface, "GetLayout", "iias", int32(9999), int32(-1), []string{}); err == nil {
		t.Error("GetLayout of an unknown parent succeeded")
	}
}

func TestTheOtherMenuMethods(t *testing.T) {
	s, b, _ := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, menu: testMenu()})
	_, root := getLayout(t, b, 0, -1)
	quit := root.children[5].id

	for name, want := range map[string]any{"Version": uint32(3), "TextDirection": "ltr", "Status": "normal",
		"IconThemePath": []any(nil)} {
		if got := b.prop(t, sniMenuPath, menuIface, name); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
	r, err := b.call(t, sniMenuPath, menuIface, "GetGroupProperties", "aias", []any{quit, int32(9999)}, []string{"label"})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{[]any{quit, []dbus.DictEntry{{Key: "label", Value: dbus.Variant{Sig: "s", Value: "Quit"}}}}}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("GetGroupProperties\n got %#v\nwant %#v", r, want)
	}
	r, err = b.call(t, sniMenuPath, menuIface, "GetGroupProperties", "aias", []any{}, []string{})
	if err != nil || len(r[0].([]any)) != 9 {
		t.Fatalf("GetGroupProperties of everything: %v %v", r, err)
	}
	r, err = b.call(t, sniMenuPath, menuIface, "GetProperty", "is", quit, "label")
	if err != nil || r[0] != (dbus.Variant{Sig: "s", Value: "Quit"}) {
		t.Fatalf("GetProperty %v %v", r, err)
	}
	if _, err := b.call(t, sniMenuPath, menuIface, "GetProperty", "is", quit, "icon-name"); err == nil {
		t.Error("GetProperty of a property that is not set succeeded")
	}
	r, err = b.call(t, sniMenuPath, menuIface, "AboutToShow", "i", int32(0))
	if err != nil || r[0] != false {
		t.Fatalf("AboutToShow %v %v", r, err)
	}
	r, err = b.call(t, sniMenuPath, menuIface, "AboutToShowGroup", "ai", []any{int32(0), int32(9999)})
	if err != nil || !reflect.DeepEqual(r, []any{[]any(nil), []any{int32(9999)}}) {
		t.Fatalf("AboutToShowGroup %#v %v", r, err)
	}
}

func waitClick(t *testing.T, clicks chan click) click {
	t.Helper()
	select {
	case c := <-clicks:
		return c
	case <-time.After(time.Second):
		t.Fatal("no click arrived")
		return click{}
	}
}

func noClick(t *testing.T, clicks chan click) {
	t.Helper()
	select {
	case c := <-clicks:
		t.Fatalf("unexpected click %+v", c)
	case <-time.After(50 * time.Millisecond):
	}
}

func event(t *testing.T, b *fakeSNIBus, id int32, kind string) error {
	t.Helper()
	_, err := b.call(t, sniMenuPath, menuIface, "Event", "isvu", id, kind, dbus.Variant{Sig: "i", Value: int32(0)}, uint32(0))
	return err
}

func TestAClickedEventReportsTheEntry(t *testing.T) {
	s, b, clicks := newTestSNI(t)
	show(t, s, state{gen: 7, icon: outline.User, menu: testMenu()})
	_, root := getLayout(t, b, 0, -1)
	c := root.children

	if err := event(t, b, c[3].children[1].id, "clicked"); err != nil {
		t.Fatal(err)
	}
	if got := waitClick(t, clicks); got != (click{7, 3}) {
		t.Fatalf("click %+v, want gen 7 id 3", got)
	}
	// Hovering is not clicking; a separator, a label, a submenu and a
	// disabled entry report nothing.
	if err := event(t, b, c[5].id, "hovered"); err != nil {
		t.Fatal(err)
	}
	for _, n := range []layoutNode{c[1], c[2], c[3], c[4]} {
		if err := event(t, b, n.id, "clicked"); err != nil {
			t.Fatal(err)
		}
	}
	noClick(t, clicks)
	if err := event(t, b, 9999, "clicked"); err == nil {
		t.Fatal("an event for an unknown item succeeded")
	}

	r, err := b.call(t, sniMenuPath, menuIface, "EventGroup", "a(isvu)", []any{
		[]any{c[5].id, "clicked", dbus.Variant{Sig: "i", Value: int32(0)}, uint32(0)},
		[]any{int32(9999), "clicked", dbus.Variant{Sig: "i", Value: int32(0)}, uint32(0)},
	})
	if err != nil || !reflect.DeepEqual(r, []any{[]any{int32(9999)}}) {
		t.Fatalf("EventGroup %v %v", r, err)
	}
	if got := waitClick(t, clicks); got != (click{7, 5}) {
		t.Fatalf("click %+v, want gen 7 id 5", got)
	}
}

// TestTheSameMenuKeepsItsIdsAndANewOneRetiresThem: a SetTooltip, which
// hands on the menu of the last Set, must not make the host rebuild the
// menu, and a click on it reports the newest generation. Every Set gets new
// ids, also one of the same entries, so that a click the host sends for the
// old menu is dropped, as on the other platforms.
func TestTheSameMenuKeepsItsIdsAndANewOneRetiresThem(t *testing.T) {
	s, b, clicks := newTestSNI(t)
	menu1 := testMenu()
	show(t, s, state{gen: 1, icon: outline.User, tooltip: "a", menu: menu1})
	b.emitted()
	rev1, root1 := getLayout(t, b, 0, -1)

	show(t, s, state{gen: 2, icon: outline.User, tooltip: "b", menu: menu1})
	if got := b.emitted(); !reflect.DeepEqual(got, []string{"NewTitle", "NewToolTip"}) {
		t.Fatalf("signals %v, want only the tooltip and title", got)
	}
	rev2, root2 := getLayout(t, b, 0, -1)
	if rev2 != rev1 || root2.children[0].id != root1.children[0].id {
		t.Fatalf("revision %d→%d, id %d→%d", rev1, rev2, root1.children[0].id, root2.children[0].id)
	}
	if err := event(t, b, root2.children[0].id, "clicked"); err != nil {
		t.Fatal(err)
	}
	if got := waitClick(t, clicks); got != (click{2, 1}) {
		t.Fatalf("click %+v, want gen 2 id 1", got)
	}

	// A Set of the same entries is a new menu.
	show(t, s, state{gen: 3, icon: outline.User, tooltip: "b", menu: testMenu()})
	if got := b.emitted(); !reflect.DeepEqual(got, []string{"LayoutUpdated"}) {
		t.Fatalf("signals %v, want LayoutUpdated", got)
	}
	rev3, root3 := getLayout(t, b, 0, -1)
	if rev3 != rev1+1 || root3.children[0].id == root1.children[0].id {
		t.Fatalf("revision %d, id %d after a Set of the same entries", rev3, root3.children[0].id)
	}
	if err := event(t, b, root1.children[0].id, "clicked"); err == nil {
		t.Fatal("a click on the replaced menu was accepted")
	}
	noClick(t, clicks)

	menu := testMenu()
	menu[5].title = "Quit now"
	show(t, s, state{gen: 4, icon: outline.CameraPhoto, tooltip: "b", menu: menu})
	b.mu.Lock()
	var layout *dbus.Message
	for _, m := range b.signals {
		if m.Member == "LayoutUpdated" {
			layout = m
		}
	}
	b.mu.Unlock()
	if got := b.emitted(); !reflect.DeepEqual(got, []string{"NewIcon", "LayoutUpdated"}) {
		t.Fatalf("signals %v", got)
	}
	rev4, root4 := getLayout(t, b, 0, -1)
	if rev4 != rev3+1 || layout.Path != sniMenuPath || !reflect.DeepEqual(layout.Body, []any{rev4, int32(0)}) {
		t.Fatalf("revision %d, LayoutUpdated %+v", rev4, layout)
	}
	if root4.children[5].props["label"] != "Quit now" {
		t.Fatalf("label %v", root4.children[5].props)
	}
	if err := event(t, b, root3.children[0].id, "clicked"); err == nil {
		t.Fatal("a click on the replaced menu was accepted")
	}
	noClick(t, clicks)
}

// TestAFloodOfRequestsIsBounded: any peer on the bus can call the menu, so
// what one call costs is bounded by the menu, not by the request.
func TestAFloodOfRequestsIsBounded(t *testing.T) {
	s, b, clicks := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, menu: testMenu()})
	_, root := getLayout(t, b, 0, -1)
	quit := root.children[5].id

	many := make([]string, menuMaxNames+1)
	if _, err := b.call(t, sniMenuPath, menuIface, "GetGroupProperties", "aias", []any{}, many); err == nil {
		t.Error("GetGroupProperties took more property names than there can be")
	}
	if _, err := b.call(t, sniMenuPath, menuIface, "GetLayout", "iias", int32(0), int32(-1), many); err == nil {
		t.Error("GetLayout took more property names than there can be")
	}
	ids := make([]any, 5000)
	for i := range ids {
		ids[i] = quit
	}
	r, err := b.call(t, sniMenuPath, menuIface, "GetGroupProperties", "aias", ids, []string{"label"})
	if err != nil || len(r[0].([]any)) != 1 {
		t.Fatalf("GetGroupProperties of one id 5000 times: %d answers, %v", len(r[0].([]any)), err)
	}

	click1 := []any{quit, "clicked", dbus.Variant{Sig: "i", Value: int32(0)}, uint32(0)}
	tooMany := make([]any, MaxItems+2)
	for i := range tooMany {
		tooMany[i] = click1
	}
	if _, err := b.call(t, sniMenuPath, menuIface, "EventGroup", "a(isvu)", tooMany); err == nil {
		t.Error("EventGroup took more events than a menu has entries")
	}
	if _, err := b.call(t, sniMenuPath, menuIface, "EventGroup", "a(isvu)", tooMany[:MaxItems]); err != nil {
		t.Fatal(err)
	}
	if got := waitClick(t, clicks); got.id != 5 {
		t.Fatalf("click %+v", got)
	}
	noClick(t, clicks)

	// Single Events beyond what the clicker queues are dropped, not
	// started each on a goroutine of its own.
	for range 10 * sniClickQueue {
		if err := event(t, b, quit, "clicked"); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for {
		select {
		case <-clicks:
			n++
			continue
		case <-time.After(100 * time.Millisecond):
		}
		break
	}
	if n == 0 || n > 10*sniClickQueue {
		t.Fatalf("%d clicks delivered", n)
	}
}

// TestAWatcherAppearingWhileTheItemIsCreatedIsNotMissed: the panel claims
// the watcher's name after AddMatch but before NameHasOwner is answered.
func TestAWatcherAppearingWhileTheItemIsCreatedIsNotMissed(t *testing.T) {
	s, b, _ := newTestSNIOn(t, &fakeSNIBus{appearOnMatch: true}, nil)
	b.mu.Lock()
	b.watcher = true // what NameHasOwner would say by now; it was asked before
	b.mu.Unlock()
	show(t, s, state{gen: 1, icon: outline.User, menu: testMenu()})
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v", got)
	}
}

// TestNameOwnerChangedFromAnotherPeerIsIgnored: only the bus says who owns
// a name.
func TestNameOwnerChangedFromAnotherPeerIsIgnored(t *testing.T) {
	s, b, _ := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, menu: testMenu()})
	forged := nameOwnerChanged("")
	forged.Sender = ":1.66"
	s.signal(forged)
	forged = nameOwnerChanged(":1.67")
	forged.Sender = ":1.66"
	s.signal(forged)
	s.flush()
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v after forged signals", got)
	}
	s.mu.Lock()
	watcher := s.watcher
	s.mu.Unlock()
	if !watcher {
		t.Fatal("a forged NameOwnerChanged hid the watcher")
	}
}

func TestRemoveReleasesTheNameAndUpdateRegistersAgain(t *testing.T) {
	s, b, _ := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User})
	if err := s.remove(); err != nil {
		t.Fatal(err)
	}
	s.flush()
	b.mu.Lock()
	owned := b.owned
	b.mu.Unlock()
	if owned {
		t.Fatal("the name is still owned after remove")
	}
	show(t, s, state{gen: 2, icon: outline.User})
	if got := b.registrations(); len(got) != 2 || got[0] != got[1] {
		t.Fatalf("registrations %v", got)
	}
}

func TestANewWatcherIsRegisteredWith(t *testing.T) {
	s, b, _ := newTestSNI(t)
	b.refuseReg = &dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}
	show(t, s, state{gen: 1, icon: outline.User})
	if got := b.registrations(); len(got) != 0 {
		t.Fatalf("registrations %v", got)
	}
	b.mu.Lock()
	b.refuseReg = nil
	b.mu.Unlock()
	s.signal(nameOwnerChanged(""))
	s.flush()
	if got := b.registrations(); len(got) != 0 {
		t.Fatalf("registered with a watcher that went away: %v", got)
	}
	s.signal(nameOwnerChanged(":1.99"))
	s.flush()
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v after the watcher came back", got)
	}
	// Someone else's name changing hands is not the watcher.
	s.signal(&dbus.Message{Type: dbus.TypeSignal, Sender: dbus.BusName, Path: dbus.BusPath,
		Interface: dbus.BusInterface, Member: "NameOwnerChanged",
		Sig: "sss", Body: []any{"org.example.Other", "", ":1.5"}})
	s.flush()
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v", got)
	}
}

func TestMenuLabelEscapesTheMnemonic(t *testing.T) {
	for in, want := range map[string]string{"a": "a", "_a": "__a", "a__b": "a____b", "": ""} {
		if got := menuLabel(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// TestARefusedRegistrationWaitsForTheNextWatcher: a watcher that refuses the
// item is not asked again on every update, which for a tooltip that shows
// progress would be a round trip and a warning a second, but when a new
// watcher appears.
func TestARefusedRegistrationWaitsForTheNextWatcher(t *testing.T) {
	log := &logRecords{}
	s, b, _ := newTestSNIOn(t, &fakeSNIBus{watcher: true, refuseReg: &dbus.Error{Name: dbus.ErrorFailed}}, slog.New(log))
	for gen := range uint64(5) {
		show(t, s, state{gen: gen + 1, icon: outline.User, tooltip: fmt.Sprint(gen)})
	}
	if n := b.count("RegisterStatusNotifierItem"); n != 1 {
		t.Fatalf("%d registrations attempted, want 1", n)
	}
	if n := log.count("WARN", "did not take"); n != 1 {
		t.Fatalf("%d warnings, want 1: %q", n, log.msgs)
	}
	b.mu.Lock()
	b.refuseReg = nil
	b.mu.Unlock()
	s.signal(nameOwnerChanged(":1.99"))
	s.flush()
	if got := b.registrations(); len(got) != 1 {
		t.Fatalf("registrations %v after a new watcher appeared", got)
	}
}

// TestAFailingSignalIsLoggedOnce: a connection that is gone fails every
// signal of every update; the log says so once, and again only after it
// worked in between.
func TestAFailingSignalIsLoggedOnce(t *testing.T) {
	log := &logRecords{}
	s, b, _ := newTestSNIOn(t, &fakeSNIBus{watcher: true}, slog.New(log))
	show(t, s, state{gen: 1, icon: outline.User, tooltip: "0"})
	b.mu.Lock()
	b.emitErr = dbus.ErrClosed
	b.mu.Unlock()
	for gen := range uint64(4) {
		show(t, s, state{gen: gen + 2, icon: outline.User, tooltip: fmt.Sprint(gen + 1)})
	}
	if n := log.count("ERROR", "signal failed"); n != 1 {
		t.Fatalf("%d errors, want 1: %q", n, log.msgs)
	}
	b.mu.Lock()
	b.emitErr = nil
	b.mu.Unlock()
	show(t, s, state{gen: 10, icon: outline.User, tooltip: "ok"})
	b.mu.Lock()
	b.emitErr = dbus.ErrClosed
	b.mu.Unlock()
	show(t, s, state{gen: 11, icon: outline.User, tooltip: "again"})
	if n := log.count("ERROR", "signal failed"); n != 2 {
		t.Fatalf("%d errors after a recovery, want 2: %q", n, log.msgs)
	}
}

// TestATakenNameTriesTheNext: in a PID namespace two applications are the
// same pid and want the same name; the second takes the next one.
func TestATakenNameTriesTheNext(t *testing.T) {
	b := &fakeSNIBus{watcher: true, taken: map[string]bool{}}
	s, _, _ := newTestSNIOn(t, b, nil)
	first := s.name
	b.taken[first] = true
	show(t, s, state{gen: 1, icon: outline.User})
	got := b.registrations()
	if len(got) != 1 || got[0] == first || !strings.HasPrefix(got[0], "org.kde.StatusNotifierItem-") {
		t.Fatalf("registered %v, want a name other than the taken %s", got, first)
	}
}

// TestTheUniqueNameIsTheFallback: when no well-known name is to be had —
// all taken, or refused by a Flatpak's proxy — the item registers under the
// unique name of its connection, and remove makes it passive instead of
// releasing a name it cannot release.
func TestTheUniqueNameIsTheFallback(t *testing.T) {
	for name, b := range map[string]*fakeSNIBus{
		"refused": {watcher: true, denyName: &dbus.Error{Name: "org.freedesktop.DBus.Error.AccessDenied"}},
		"taken":   {watcher: true, taken: map[string]bool{}},
	} {
		t.Run(name, func(t *testing.T) {
			if b.taken != nil {
				// The next names of this process, which the item will try.
				next := sniSerial.Load()
				for i := range uint32(sniNameTries + 1) {
					b.taken[fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), next+1+i)] = true
				}
			}
			s, _, _ := newTestSNIOn(t, b, nil)
			show(t, s, state{gen: 1, icon: outline.User})
			if got := b.registrations(); len(got) != 1 || got[0] != fakeUniqueName {
				t.Fatalf("registered %v, want the unique name %s", got, fakeUniqueName)
			}
			b.emitted()
			if err := s.remove(); err != nil {
				t.Fatal(err)
			}
			s.flush()
			if n := b.count("ReleaseName"); n != 0 {
				t.Fatalf("released a unique name %d times", n)
			}
			if got := b.prop(t, sniItemPath, sniItemIface, "Status"); got != "Passive" {
				t.Fatalf("Status %v after remove, want Passive", got)
			}
			b.mu.Lock()
			sigs := slices.Clone(b.signals)
			b.mu.Unlock()
			if len(sigs) != 1 || sigs[0].Member != "NewStatus" || !reflect.DeepEqual(sigs[0].Body, []any{"Passive"}) {
				t.Fatalf("signals %+v, want NewStatus(Passive)", sigs)
			}
			b.emitted()
			show(t, s, state{gen: 2, icon: outline.User})
			if got := b.prop(t, sniItemPath, sniItemIface, "Status"); got != "Active" {
				t.Fatalf("Status %v after update, want Active", got)
			}
			if got := b.emitted(); !reflect.DeepEqual(got, []string{"NewStatus"}) {
				t.Fatalf("signals %v, want NewStatus", got)
			}
			if got := b.registrations(); len(got) != 1 {
				t.Fatalf("registrations %v: the watcher still knows the item", got)
			}
		})
	}
}

// TestADisabledSubmenuIsClosed: its entries are numbered by the portable
// half like any other, but a click on one, from a host that opened the
// submenu anyway, is not an OnClick.
func TestADisabledSubmenuIsClosed(t *testing.T) {
	s, b, clicks := newTestSNI(t)
	show(t, s, state{gen: 1, icon: outline.User, menu: []entry{
		{title: "Off", disabled: true, children: []entry{
			{id: 1, title: "Inside"},
			{title: "Deeper", children: []entry{{id: 2, title: "Deepest"}}},
		}},
		{id: 3, title: "On"},
	}})
	_, root := getLayout(t, b, 0, -1)
	off := root.children[0]
	for _, n := range []layoutNode{off.children[0], off.children[1].children[0]} {
		if err := event(t, b, n.id, "clicked"); err != nil {
			t.Fatal(err)
		}
	}
	noClick(t, clicks)
	if err := event(t, b, root.children[1].id, "clicked"); err != nil {
		t.Fatal(err)
	}
	if got := waitClick(t, clicks); got != (click{1, 3}) {
		t.Fatalf("click %+v, want gen 1 id 3", got)
	}
}
