package tray

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"slices"
	"strings"

	"github.com/worldiety/gift/internal/dbus"
	"github.com/worldiety/gift/ui"
)

// This file is the menu of the Linux tray: the com.canonical.dbusmenu
// object at /MenuBar that the StatusNotifierItem of sni.go names in its Menu
// property. Like sni.go it has no build tag and is tested everywhere.
//
// # The protocol
//
// dbusmenu was written by Canonical for the indicators of Unity and is what
// every StatusNotifierItem host reads a menu with; there is no other. The
// menu is a tree of items, each an int32 id and a dictionary of properties,
// with the root at id 0. The host asks for it with GetLayout and draws it
// itself — the application never sees the menu open — and reports a click
// with Event(id, "clicked", …). The application announces a new tree with
// LayoutUpdated(revision, 0), after which the host asks again.
//
// Version 3 of the protocol is spoken, the one libdbusmenu and Qt's
// importer implement.
//
// # Ids
//
// Every node gets an id of its own, separators and submenus included, from a
// counter that keeps counting across menus rather than starting at 1 for
// each. That is what makes a stale click harmless: a host that sends Event
// for an id of the menu before the last Set names a node that no longer
// exists, and the click is dropped here, before the portable half would drop
// it by its generation. The map from node to [entry.id] is part of each
// published menu.
//
// Every Set publishes a new tree with new ids and a LayoutUpdated, also one
// of the same entries: a click in the menu it replaced is dropped, as the
// contract of the package says and the other platforms do. SetIcon and
// SetTooltip keep the published tree, its ids and its revision, and send no
// LayoutUpdated, so that the host does not rebuild a menu that may be open
// while a tooltip counts; see [sni.publish].
//
// # Requests from the bus
//
// Any peer on the session bus can call these methods, not only the host, so
// the work one call can cause is bounded by the menu rather than by the
// request: GetGroupProperties answers each existing id once, however often
// the request names it, a list of property names longer than
// [menuMaxNames] is refused, and EventGroup takes at most [MaxItems]+1
// events and hands on at most one click.
//
// # Item properties
//
// Only those that differ from the defaults of the specification are sent,
// as the specification allows:
//
//	type "separator"            for a separator; the default is "standard"
//	label                       the title, with "_" doubled: a single one
//	                            marks the mnemonic of the next letter
//	enabled false               a disabled entry
//	toggle-type "checkmark",    a checked entry; an unchecked one has no
//	toggle-state 1              toggle at all, see [Item.Checked]
//	children-display "submenu"  an entry with a submenu, and the root
//	icon-data                   the symbol as PNG bytes
//
// # Menu icons
//
// icon-data is one picture, which the host scales to the size it draws menu
// icons at: 16 logical pixels in Plasma and in GNOME. It is rasterised at
// 32, which is that size at a scale of 2 and halves cleanly at a scale of 1.
// The host does not recolour it, and the menu is light or dark with the
// theme, so it is a mid grey that reads on both, rather than the outlined
// white of the icon in the bar: a menu, unlike a panel, has the colours of
// the windows.

const menuIface = "com.canonical.dbusmenu"

// menuIconPx is the edge of icon-data in pixels; menuIconColor its colour.
const menuIconPx = 32

var menuIconColor = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}

// menuMaxNames bounds the property names a request may ask for. There are
// seven; libdbusmenu and Qt ask for none, which means all of them.
const menuMaxNames = 64

// menuNode is one item of a published menu.
type menuNode struct {
	id       int32
	props    map[string]dbus.Variant
	children []*menuNode

	// entry is the [entry.id] a click on this node reports, 0 for one that
	// cannot be clicked: a separator, a submenu, a disabled entry, an entry
	// in the submenu of a disabled one, one without OnClick.
	entry uint32
}

// menuLayout is a published menu: the tree and its nodes by id. It is never
// changed after it is built.
type menuLayout struct {
	root  *menuNode
	nodes map[int32]*menuNode
}

func emptyMenu() *menuLayout {
	return buildMenu(nil, new(int32))
}

// buildMenu turns entries into a tree, numbering its nodes from *next on.
func buildMenu(entries []entry, next *int32) *menuLayout {
	l := &menuLayout{nodes: map[int32]*menuNode{}}
	// A counter that would run past int32 within this menu starts again.
	// Two billion nodes after the first, a stale click could hit a reused
	// id; the portable half's generation check still drops it.
	if *next < 1 || *next > math.MaxInt32-MaxItems-1 {
		*next = 1
	}
	l.root = &menuNode{id: 0, props: map[string]dbus.Variant{
		"children-display": {Sig: "s", Value: "submenu"},
	}}
	l.nodes[0] = l.root
	// off is true below a disabled entry. Its submenu cannot be opened, the
	// contract says; a host that honours enabled=false never shows the
	// entries in it, and a host that does not, or a stale or forged Event,
	// must not reach their OnClick either. The portable half numbers them
	// all the same, so the defence is here, as for a disabled leaf.
	var add func(parent *menuNode, es []entry, off bool)
	add = func(parent *menuNode, es []entry, off bool) {
		for _, e := range es {
			n := &menuNode{id: *next, props: menuProps(e)}
			*next++
			if !off && !e.separator && !e.disabled && len(e.children) == 0 {
				n.entry = e.id
			}
			l.nodes[n.id] = n
			parent.children = append(parent.children, n)
			if len(e.children) > 0 {
				add(n, e.children, off || e.disabled)
			}
		}
	}
	add(l.root, entries, false)
	return l
}

// menuProps are the properties of e that differ from the defaults.
func menuProps(e entry) map[string]dbus.Variant {
	p := map[string]dbus.Variant{}
	if e.separator {
		p["type"] = dbus.Variant{Sig: "s", Value: "separator"}
		return p
	}
	p["label"] = dbus.Variant{Sig: "s", Value: menuLabel(e.title)}
	if e.disabled {
		p["enabled"] = dbus.Variant{Sig: "b", Value: false}
	}
	if e.checked {
		p["toggle-type"] = dbus.Variant{Sig: "s", Value: "checkmark"}
		p["toggle-state"] = dbus.Variant{Sig: "i", Value: int32(1)}
	}
	if len(e.children) > 0 {
		p["children-display"] = dbus.Variant{Sig: "s", Value: "submenu"}
	}
	if b := menuIcon(e.symbol); b != nil {
		p["icon-data"] = dbus.Variant{Sig: "ay", Value: b}
	}
	return p
}

// menuLabel escapes the mnemonic marker: dbusmenu, like GTK, reads "_x" as
// "x, underlined, and Alt+x selects it", and "__" as one underscore.
func menuLabel(title string) string {
	return strings.ReplaceAll(title, "_", "__")
}

// menuIcon is the PNG of sym for icon-data, or nil for a symbol that draws
// nothing.
func menuIcon(sym ui.Symbol) []byte {
	im := tinted(sym, menuIconPx, menuIconColor)
	if im == nil {
		return nil
	}
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, im); err != nil {
		return nil
	}
	return b.Bytes()
}

// value is the node as the (ia{sv}av) of GetLayout, with depth levels of
// children: -1 for all of them, 0 for none.
func (n *menuNode) value(depth int32, names map[string]bool) []any {
	children := []any{}
	if depth != 0 {
		for _, c := range n.children {
			children = append(children, dbus.Variant{Sig: "(ia{sv}av)", Value: c.value(depth-1, names)})
		}
	}
	return []any{n.id, n.dict(names), children}
}

// dict is the a{sv} of the node's properties, sorted, limited to names if
// names is not empty.
func (n *menuNode) dict(names map[string]bool) []dbus.DictEntry {
	keys := make([]string, 0, len(n.props))
	for k := range n.props {
		if len(names) == 0 || names[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := make([]dbus.DictEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, dbus.DictEntry{Key: k, Value: n.props[k]})
	}
	return out
}

// published returns the menu the host is shown, its revision and the
// generation of the state it was made from.
func (s *sni) published() (*menuLayout, uint32, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.menu, s.revision, s.gen
}

func noItem(id int32) error {
	return &dbus.Error{Name: dbus.ErrorInvalidArgs, Message: fmt.Sprintf("there is no menu item %d", id)}
}

// event handles one Event: a click on a clickable node is queued for the
// clicker ([sni.click]), because this runs on the reader goroutine of the
// connection and the seam forbids calling back from where the platform was
// called. The other events — "opened", "closed", "hovered" — need no
// answer. It reports whether the event was a click that was handed on.
func (s *sni) event(id int32, kind string, clickOK bool) (bool, error) {
	l, _, gen := s.published()
	n, ok := l.nodes[id]
	if !ok {
		return false, noItem(id)
	}
	if kind == "clicked" && n.entry != 0 && clickOK {
		s.click(gen, n.entry)
		return true, nil
	}
	return false, nil
}

// propertyNames is the as of a request as a set, refused when it is longer
// than [menuMaxNames].
func propertyNames(v any) (map[string]bool, error) {
	list := v.([]any)
	if len(list) > menuMaxNames {
		return nil, &dbus.Error{Name: dbus.ErrorInvalidArgs,
			Message: fmt.Sprintf("more than %d property names", menuMaxNames)}
	}
	out := make(map[string]bool, len(list))
	for _, x := range list {
		out[x.(string)] = true
	}
	return out, nil
}

// menuInterface is com.canonical.dbusmenu over the published menu.
func (s *sni) menuInterface() *dbus.Interface {
	constant := func(sig dbus.Signature, v any) dbus.Property {
		return dbus.Property{Sig: sig, Get: func() any { return v }}
	}
	return &dbus.Interface{
		Name: menuIface,
		Properties: map[string]dbus.Property{
			"Version":       constant("u", uint32(3)),
			"TextDirection": constant("s", "ltr"),
			"Status":        constant("s", "normal"),
			"IconThemePath": constant("as", []string{}),
		},
		Methods: map[string]dbus.Method{
			"GetLayout": {In: "iias", Out: "u(ia{sv}av)", Call: func(m *dbus.Message) ([]any, error) {
				parent, depth := m.Body[0].(int32), m.Body[1].(int32)
				names, err := propertyNames(m.Body[2])
				if err != nil {
					return nil, err
				}
				l, rev, _ := s.published()
				n, ok := l.nodes[parent]
				if !ok {
					return nil, noItem(parent)
				}
				return []any{rev, n.value(depth, names)}, nil
			}},
			"GetGroupProperties": {In: "aias", Out: "a(ia{sv})", Call: func(m *dbus.Message) ([]any, error) {
				want, err := propertyNames(m.Body[1])
				if err != nil {
					return nil, err
				}
				l, _, _ := s.published()
				// Each existing node once, in the order asked for: the
				// answer is never longer than the menu.
				var ids []int32
				seen := map[int32]bool{}
				for _, x := range m.Body[0].([]any) {
					id := x.(int32)
					if _, ok := l.nodes[id]; ok && !seen[id] {
						seen[id] = true
						ids = append(ids, id)
					}
				}
				if len(m.Body[0].([]any)) == 0 {
					for id := range l.nodes {
						ids = append(ids, id)
					}
					slices.Sort(ids)
				}
				out := []any{}
				for _, id := range ids {
					out = append(out, []any{id, l.nodes[id].dict(want)})
				}
				return []any{out}, nil
			}},
			"GetProperty": {In: "is", Out: "v", Call: func(m *dbus.Message) ([]any, error) {
				id, name := m.Body[0].(int32), m.Body[1].(string)
				l, _, _ := s.published()
				n, ok := l.nodes[id]
				if !ok {
					return nil, noItem(id)
				}
				v, ok := n.props[name]
				if !ok {
					return nil, &dbus.Error{Name: dbus.ErrorInvalidArgs,
						Message: fmt.Sprintf("menu item %d has no property %q", id, name)}
				}
				return []any{v}, nil
			}},
			"Event": {In: "isvu", Call: func(m *dbus.Message) ([]any, error) {
				_, err := s.event(m.Body[0].(int32), m.Body[1].(string), true)
				return nil, err
			}},
			"EventGroup": {In: "a(isvu)", Out: "ai", Call: func(m *dbus.Message) ([]any, error) {
				events := m.Body[0].([]any)
				if len(events) > MaxItems+1 {
					return nil, &dbus.Error{Name: dbus.ErrorInvalidArgs,
						Message: fmt.Sprintf("more than %d events", MaxItems+1)}
				}
				// One person clicks one entry; a group with more clicks
				// hands on the first.
				bad := []any{}
				clickOK := true
				for _, x := range events {
					ev := x.([]any)
					clicked, err := s.event(ev[0].(int32), ev[1].(string), clickOK)
					if err != nil {
						bad = append(bad, ev[0].(int32))
					}
					if clicked {
						clickOK = false
					}
				}
				if len(events) > 0 && len(bad) == len(events) {
					return nil, &dbus.Error{Name: dbus.ErrorInvalidArgs, Message: "none of the menu items exists"}
				}
				return []any{bad}, nil
			}},
			"AboutToShow": {In: "i", Out: "b", Call: func(m *dbus.Message) ([]any, error) {
				l, _, _ := s.published()
				if _, ok := l.nodes[m.Body[0].(int32)]; !ok {
					return nil, noItem(m.Body[0].(int32))
				}
				// The menu is complete when it is published; there is
				// nothing to fill in before it opens.
				return []any{false}, nil
			}},
			"AboutToShowGroup": {In: "ai", Out: "aiai", Call: func(m *dbus.Message) ([]any, error) {
				l, _, _ := s.published()
				ids := m.Body[0].([]any)
				if len(ids) > MaxItems+1 {
					return nil, &dbus.Error{Name: dbus.ErrorInvalidArgs,
						Message: fmt.Sprintf("more than %d ids", MaxItems+1)}
				}
				bad := []any{}
				for _, x := range ids {
					if _, ok := l.nodes[x.(int32)]; !ok {
						bad = append(bad, x.(int32))
					}
				}
				return []any{[]any{}, bad}, nil
			}},
		},
		Signals: map[string]dbus.Signature{
			"LayoutUpdated":           "ui",
			"ItemsPropertiesUpdated":  "a(ia{sv})a(ias)",
			"ItemActivationRequested": "iu",
		},
	}
}
