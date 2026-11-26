package ui

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	menuType        = gift.RegisterType("ui.Menu")
	menuButtonType  = gift.RegisterType("ui.menuButton")
	menuListType    = gift.RegisterType("ui.menuList")
	menuRowType     = gift.RegisterType("ui.menuRow")
	contextMenuType = gift.RegisterType("ui.ContextMenu")
	contextHostType = gift.RegisterType("ui.contextHost")
)

// Metrics of a menu, in logical pixels: the ones of iOS.
const (
	menuWidth      = float32(250)
	menuRowHeight  = float32(44)
	menuHeader     = float32(32)
	menuSectionGap = float32(13)
	menuListPad    = float32(6)
	menuPadding    = float32(16)
	menuIcon       = float32(20)
	menuCheck      = float32(28)
	menuTitleSize  = float32(17)
	menuHeaderSize = float32(13)
)

// MenuElement is an entry of a menu: a [MenuItem], a [MenuDivider] or a
// [MenuSection].
type MenuElement interface{ menuElement() }

// MenuItemView is one action of a menu. It is created by [MenuItem].
type MenuItemView struct {
	title       string
	action      func()
	icon        Symbol
	destructive bool
	disabled    bool
	checked     bool
	key         string
}

// MenuItem returns an item that runs action when it is chosen. The menu
// closes first, so action may present something else.
func MenuItem(title string, action func()) MenuItemView {
	return MenuItemView{title: title, action: action}
}

// Icon sets the symbol shown at the trailing end of the item.
func (m MenuItemView) Icon(s Symbol) MenuItemView { m.icon = s; return m }

// Destructive draws the item in [ColorDanger], for an action that deletes or
// discards.
func (m MenuItemView) Destructive() MenuItemView { m.destructive = true; return m }

// Disabled shows the item and refuses to choose it.
func (m MenuItemView) Disabled(b bool) MenuItemView { m.disabled = b; return m }

// Checked puts a check mark in front of the item: the current choice of a
// menu that picks one of several.
func (m MenuItemView) Checked(b bool) MenuItemView { m.checked = b; return m }

// Key sets the key of the item's row, for a test to find it by. The default is
// "menu-" followed by the title.
func (m MenuItemView) Key(s string) MenuItemView { m.key = s; return m }

func (MenuItemView) menuElement() {}

type menuDivider struct{}

func (menuDivider) menuElement() {}

// MenuDivider returns a hairline between two items.
func MenuDivider() MenuElement { return menuDivider{} }

type menuSection struct {
	title string
	items []MenuElement
}

func (menuSection) menuElement() {}

// MenuSection returns items that belong together, set apart from the rest of
// the menu by a hairline with room above and below it, under title when it is
// not empty.
func MenuSection(title string, items ...MenuElement) MenuElement {
	return menuSection{title: title, items: items}
}

// --- the shared state of an open menu ---------------------------------------

// menuKind is what a row of an open menu is.
type menuKind uint8

const (
	menuKindItem menuKind = iota
	menuKindDivider
	menuKindGap
	menuKindHeader
)

type menuRowSpec struct {
	kind  menuKind
	item  MenuItemView
	title string
	index int // the item's index among the items, for an item
}

// menuState is what the presenter of a menu and the menu itself share: the
// items, the one under the finger, and where each row was drawn, so that a
// finger that pressed the button and slides onto the menu can choose an item
// it never pressed — the gesture of iOS. It is kept in the state of the
// presenter's component, so it survives every build.
type menuState struct {
	items     []MenuItemView
	rows      []menuRowSpec
	rects     []geom.Rect // device rectangle of each item, as last drawn
	highlight int
	checks    bool // some item has a check mark, so every item makes room

	pressed bool // the finger that opened the menu is still down
	dragged bool
	downPos geom.Point

	open, close func()
	activated   bool
}

func newMenuState() *menuState { return &menuState{highlight: -1} }

// setItems flattens the elements into rows.
func (st *menuState) setItems(es []MenuElement) {
	st.items, st.rows, st.checks = st.items[:0], st.rows[:0], false
	var add func(es []MenuElement, top bool)
	add = func(es []MenuElement, top bool) {
		for _, e := range es {
			switch e := e.(type) {
			case MenuItemView:
				st.rows = append(st.rows, menuRowSpec{kind: menuKindItem, item: e, index: len(st.items)})
				st.items = append(st.items, e)
				st.checks = st.checks || e.checked
			case menuDivider:
				st.rows = append(st.rows, menuRowSpec{kind: menuKindDivider})
			case menuSection:
				if len(st.rows) > 0 {
					st.rows = append(st.rows, menuRowSpec{kind: menuKindGap})
				}
				if e.title != "" {
					st.rows = append(st.rows, menuRowSpec{kind: menuKindHeader, title: e.title})
				}
				add(e.items, false)
			}
		}
	}
	add(es, true)
	if cap(st.rects) < len(st.items) {
		st.rects = make([]geom.Rect, len(st.items))
	}
	st.rects = st.rects[:len(st.items)]
	clear(st.rects)
}

// itemAt is the enabled item under the device position p, or -1.
func (st *menuState) itemAt(p geom.Point) int {
	for i, r := range st.rects {
		if r.Contains(p) && !st.items[i].disabled {
			return i
		}
	}
	return -1
}

// activate chooses item i: the menu closes and the action runs.
func (st *menuState) activate(i int) {
	if i < 0 || i >= len(st.items) || st.items[i].disabled {
		return
	}
	st.activated = true
	if st.close != nil {
		st.close()
	}
	if a := st.items[i].action; a != nil {
		a()
	}
}

// track follows the finger that opened the menu: past the slop it highlights
// the item under it.
func (st *menuState) track(p geom.Point) {
	if !st.pressed {
		return
	}
	if !st.dragged {
		d := p.Sub(st.downPos)
		if d.X*d.X+d.Y*d.Y < gift.DragSlop*gift.DragSlop {
			return
		}
		st.dragged = true
	}
	st.highlight = st.itemAt(p)
}

// release ends the finger that opened the menu: lifted on an item after
// sliding, it chooses that item; lifted after sliding elsewhere, it closes
// the menu; lifted where it went down, it leaves the menu open for a tap.
func (st *menuState) release(p geom.Point) {
	if !st.pressed {
		return
	}
	st.pressed = false
	if !st.dragged {
		return
	}
	if i := st.itemAt(p); i >= 0 {
		st.activate(i)
		return
	}
	if st.close != nil {
		st.close()
	}
}

// key moves the highlight with the arrow keys and chooses with enter or
// space.
func (st *menuState) key(e gift.Event) bool {
	if e.Kind != gift.EventKeyDown {
		return false
	}
	step := func(dir int) {
		n := len(st.items)
		for k := 1; k <= n; k++ {
			i := ((st.highlight+dir*k)%n + n) % n
			if st.highlight < 0 && dir < 0 {
				i = n - k
			} else if st.highlight < 0 {
				i = k - 1
			}
			if !st.items[i].disabled {
				st.highlight = i
				return
			}
		}
	}
	switch e.Key {
	case gift.KeyDown:
		step(1)
	case gift.KeyUp:
		step(-1)
	case gift.KeyHome:
		st.highlight = -1
		step(1)
	case gift.KeyEnd:
		st.highlight = -1
		step(-1)
	case gift.KeyEnter, gift.KeySpace:
		if !e.Repeat {
			st.activate(st.highlight)
		}
	default:
		return false
	}
	return true
}

// menuPanel is the overlay of an open menu.
func menuPanel(st *menuState, spotlight bool, point geom.Point, hasPoint bool, radius float32) panel {
	scrim := Color{}
	if spotlight {
		scrim = RGBA(0, 0, 0, 90)
	}
	return panel{
		content:    menuList{st: st},
		edge:       EdgeAuto,
		align:      alignLeading,
		point:      point,
		hasPoint:   hasPoint,
		background: ResolveColor(ColorSurface),
		scrim:      scrim,
		spotlight:  spotlight,
		holeRadius: radius,
		maxWidth:   menuWidth,
		label:      "Menu",
		onDismiss:  st.close,
		onKey:      st.key,
	}
}

// --- the list of an open menu ------------------------------------------------

type menuList struct{ st *menuState }

func (menuList) ViewType() gift.TypeID { return menuListType }

func (l menuList) Build(*gift.BuildContext) gift.Element {
	st := l.st
	label := ResolveColor(ColorLabel)
	danger := ResolveColor(ColorDanger)
	off := ResolveColor(ColorSecondaryLabel)
	kids := make([]gift.View, 0, len(st.rows))
	for _, r := range st.rows {
		switch r.kind {
		case menuKindItem:
			c := label
			if r.item.destructive {
				c = danger
			}
			if r.item.disabled {
				c = off
			}
			kids = append(kids, menuRow{st: st, spec: r, color: c})
		case menuKindHeader:
			kids = append(kids, Text(r.title).FontSize(menuHeaderSize).Foreground(off).MaxLines(1).
				PaddingInsets(geom.Insets{Left: menuPadding, Right: menuPadding, Top: 12, Bottom: 4}))
		default:
			kids = append(kids, menuRule{gap: r.kind == menuKindGap})
		}
	}
	return gift.Element{Key: "ui.menuList", Layouter: menuListLayout{}, Children: kids}
}

// menuListLayout stacks the rows at the menu's width, with room above the
// first and below the last for the inset highlight; see [menuRowNode.Paint].
type menuListLayout struct{}

func (menuListLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	w := min(menuWidth, c.Max.W)
	y := menuListPad
	for i := range ctx.ChildCount() {
		s := ctx.Measure(i, geom.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, geom.Unbounded())})
		ctx.Place(i, geom.Pt(0, y))
		y += s.H
	}
	ctx.ReportOverflow(geom.Size{})
	return c.Constrain(geom.Sz(w, y+menuListPad))
}

var menuRuleType = gift.RegisterType("ui.menuRule")

// menuRule is a hairline between two items, or the space between two
// sections with a hairline across its middle: the separators of iOS 26,
// inset by the padding of the rows. The band of earlier versions of iOS, a
// strip of the separator colour eight points tall, read as a heavy bar on a
// light menu.
type menuRule struct{ gap bool }

func (menuRule) ViewType() gift.TypeID { return menuRuleType }

func (r menuRule) Build(*gift.BuildContext) gift.Element {
	h := float32(1)
	if r.gap {
		h = menuSectionGap
	}
	n := &ruleNode{h: h, c: ResolveColor(ColorSeparator)}
	return gift.Element{Layouter: n, Painter: n}
}

type ruleNode struct {
	h       float32
	c       Color
	density float32
}

func (n *ruleNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	n.density = ctx.Density()
	return c.Constrain(geom.Sz(c.Max.W, n.h))
}

// Paint draws half a point, which is one device pixel on a panel of twice the
// density and, snapped, one on the Pi.
func (n *ruleNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	mid := roundf(b.Min.Y + (b.Height()-0.5)/2)
	top, th := snapHairline(mid, 0.5, b.Min.Y, ctx.DeviceBounds().Min.Y, n.density)
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(b.Min.X+menuPadding, top, b.Max.X-menuPadding, top+th), Color: n.c})
}

// menuRow is one item: a check mark column when the menu has one, the title,
// and the icon at the trailing end.
type menuRow struct {
	st    *menuState
	spec  menuRowSpec
	color Color
}

func (menuRow) ViewType() gift.TypeID { return menuRowType }

func (r menuRow) Build(*gift.BuildContext) gift.Element {
	n := &menuRowNode{r: r}
	key := r.spec.item.key
	if key == "" {
		key = "menu-" + r.spec.item.title
	}
	kids := []gift.View{Text(r.spec.item.title).FontSize(menuTitleSize).Foreground(r.color).MaxLines(1)}
	if !r.spec.item.icon.IsZero() {
		kids = append(kids, Icon(r.spec.item.icon).Size(menuIcon).Foreground(r.color))
	}
	return gift.Element{
		Key:        key,
		Label:      r.spec.item.title,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Children:   kids,
		Disabled:   r.spec.item.disabled,
	}
}

type menuRowNode struct {
	r       menuRow
	density float32
}

func (n *menuRowNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	n.density = ctx.Density()
	w := c.Max.W
	left := menuPadding
	if n.r.st.checks {
		left = menuCheck + 8
	}
	right := menuPadding
	if ctx.ChildCount() > 1 {
		ctx.Measure(1, geom.Tight(geom.Sz(menuIcon, menuIcon)))
		ctx.Place(1, geom.Pt(w-menuPadding-menuIcon, (menuRowHeight-menuIcon)/2))
		right += menuIcon + 12
	}
	s := ctx.Measure(0, geom.Constraints{Max: geom.Sz(max(0, w-left-right), menuRowHeight)})
	ctx.Place(0, geom.Pt(left, (menuRowHeight-s.H)/2))
	return c.Constrain(geom.Sz(w, menuRowHeight))
}

func (n *menuRowNode) Paint(ctx *gift.PaintContext) {
	st, i := n.r.st, n.r.spec.index
	if i < len(st.rects) {
		// Bookkeeping for the finger of the presenter, as a gallery
		// records its tiles; nothing that is drawn depends on it. The laid
		// out rectangle and not the device one: the menu is drawn scaled
		// while it grows, and the finger aims at where it will be. The top
		// layer has no scroll offset, so the two spaces are one.
		st.rects[i] = ctx.Bounds()
	}
	b := ctx.Bounds()
	ia := ctx.Interaction()
	if !n.r.spec.item.disabled && (st.highlight == i || ia.Pressed || ia.Hover) {
		// Inset and rounded as on iOS 26, concentric with the corners of
		// the card, so that the first and the last row stay inside them.
		hl := geom.Rc(b.Min.X+menuListPad, b.Min.Y, b.Max.X-menuListPad, b.Max.Y)
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: hl, Color: ResolveColor(Fade(ColorLabel, 0.1)), CornerRadius: panelRadius - menuListPad})
	}
	if n.r.spec.item.checked {
		paintCheck(ctx, geom.Pt(b.Min.X+menuPadding+5, (b.Min.Y+b.Max.Y)/2), n.r.color)
	}
	ctx.PaintChildren()
}

// paintCheck draws a check mark centred on c out of two turned bars.
func paintCheck(ctx *gift.PaintContext, c geom.Point, col Color) {
	bar := func(x0, y0, x1, y1 float32) {
		dx, dy := x1-x0, y1-y0
		l := float32(math.Hypot(float64(dx), float64(dy)))
		m := geom.Pt((x0+x1)/2, (y0+y1)/2)
		ang := float32(math.Atan2(float64(dy), float64(dx)))
		ctx.PushTransform(geom.Translate(geom.Pt(-m.X, -m.Y)).Mul(geom.Rotate(ang)).Mul(geom.Translate(m)))
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(m.X-l/2, m.Y-1.1, m.X+l/2, m.Y+1.1), Color: col, CornerRadius: 1.1})
		ctx.PopTransform()
	}
	bar(c.X-6, c.Y, c.X-2, c.Y+4.5)
	bar(c.X-2, c.Y+4.5, c.X+6, c.Y-5)
}

// HandleEvent chooses the item on a tap and highlights it under a mouse.
func (n *menuRowNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st, i := n.r.st, n.r.spec.index
	switch e.Kind {
	case gift.EventPointerEnter, gift.EventPointerMove:
		if e.Device == gift.PointerMouse && st.highlight != i {
			st.highlight = i
			ctx.Repaint()
		}
		return false
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			st.activate(i)
		}
		return true
	}
	return false
}

// --- the menu of a button ------------------------------------------------------

// MenuView is a control that opens a menu. It is created by [Menu].
type MenuView struct {
	label    gift.View
	items    []MenuElement
	key      string
	name     string
	disabled bool
	flex     float32
}

// Menu returns label as a control that opens a menu of items, the
// counterpart of SwiftUI's Menu.
//
//	ui.Menu(ui.Icon(outline.DotsHorizontal),
//		ui.MenuItem("Share", share).Icon(outline.Share),
//		ui.MenuSection("",
//			ui.MenuItem("Rename", rename),
//			ui.MenuItem("Delete", remove).Destructive(),
//		),
//	)
//
// The menu opens as soon as a finger or the mouse presses the label, below it
// or above where there is room, lined up with its leading edge. A finger that
// slides on onto an item and lifts there chooses it, without a second tap; a
// press that lifts where it went down leaves the menu open for a tap on an
// item. A press outside closes the menu, and so does escape; the arrow keys,
// enter and space choose with the keyboard. The menu lives in the top layer;
// see [gift.Element.Overlay]. It is kept inside the part of the window the
// system bars of a phone leave free, [gift.App.SafeRect]: a menu of a button
// just above the navigation bar opens above the button, and none opens under
// the status bar or into the cutout of a phone on its side.
func Menu(label gift.View, items ...MenuElement) MenuView {
	return MenuView{label: label, items: items}
}

// Key sets the reconciliation key.
func (v MenuView) Key(s string) MenuView { v.key = s; return v }

// Flex sets the flex factor of the label in a stack.
func (v MenuView) Flex(f float32) MenuView { v.flex = f; return v }

// Label sets the accessible name of the control.
func (v MenuView) Label(s string) MenuView { v.name = s; return v }

// Disabled shows the label and does not open the menu.
func (v MenuView) Disabled(b bool) MenuView { v.disabled = b; return v }

// ViewType implements gift.View.
func (MenuView) ViewType() gift.TypeID { return menuType }

// Build implements gift.View.
func (v MenuView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.menu", v.body)}, Flex: v.flex}
}

// menuStateOf is the menu state of a component, created on its first build.
func menuStateOf(ctx *gift.Context) (*menuState, *gift.State[bool]) {
	open := ctx.State("open", false)
	ctx.Read(open)
	slot := ctx.State("menu", (*menuState)(nil))
	st := slot.Get()
	if st == nil {
		st = newMenuState()
		slot.Set(st)
	}
	st.open = func() {
		st.highlight, st.activated = -1, false
		open.Set(true)
	}
	st.close = func() {
		st.pressed = false
		open.Set(false)
	}
	return st, open
}

func (v MenuView) body(ctx *gift.Context) gift.View {
	st, open := menuStateOf(ctx)
	st.setItems(v.items)
	return menuButton{label: v.label, st: st, open: open.Get(), name: v.name, disabled: v.disabled}
}

type menuButton struct {
	label    gift.View
	st       *menuState
	open     bool
	name     string
	disabled bool
}

func (menuButton) ViewType() gift.TypeID { return menuButtonType }

func (b menuButton) Build(*gift.BuildContext) gift.Element {
	n := &menuButtonNode{b: b}
	e := gift.Element{
		Key:        "ui.menuButton",
		Label:      b.name,
		Layouter:   anchorLayout{},
		Interactor: n,
		Focusable:  true,
		Disabled:   b.disabled,
		Children:   []gift.View{b.label},
	}
	if b.open {
		e.Overlay = menuPanel(b.st, false, geom.Point{}, false, 0)
		e.OverlayExit = panelExit
	}
	return e
}

type menuButtonNode struct{ b menuButton }

func (n *menuButtonNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := n.b.st
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		st.pressed, st.dragged, st.downPos = true, false, e.Pos
		if !n.b.open {
			st.open()
		}
		return true
	case gift.EventPointerMove:
		if st.pressed {
			st.track(e.Pos)
			ctx.Repaint()
			return true
		}
	case gift.EventPointerUp:
		st.release(e.Pos)
		return true
	case gift.EventPointerCancel:
		st.pressed = false
		return true
	case gift.EventKeyDown:
		if (e.Key == gift.KeyEnter || e.Key == gift.KeySpace) && !e.Repeat && !n.b.open {
			st.open()
			st.highlight = -1
			st.key(gift.Event{Kind: gift.EventKeyDown, Key: gift.KeyDown})
			return true
		}
	}
	return false
}

// --- the context menu ----------------------------------------------------------

// ContextMenuView is content with a context menu. It is created by
// [ContextMenu].
type ContextMenuView struct {
	content gift.View
	items   []MenuElement
	key     string
	flex    float32
	radius  float32
}

// ContextMenu returns content with a menu of items that a long press opens on
// a touch screen and a click of the secondary mouse button on a desktop, the
// counterpart of SwiftUI's .contextMenu.
//
// After a long press the rest of the window dims and the menu appears below
// the content, or above it where there is room, as on iOS; the finger can
// slide on onto an item and lift there to choose it. A secondary click opens
// the menu at the pointer, without dimming anything, as on a desktop. The
// long press takes the finger from whatever content captured it — a button
// inside is told its press was cancelled — so the release is not also a tap;
// see [gift.EventLongPress].
func ContextMenu(content gift.View, items ...MenuElement) ContextMenuView {
	return ContextMenuView{content: content, items: items}
}

// Key sets the reconciliation key.
func (v ContextMenuView) Key(s string) ContextMenuView { v.key = s; return v }

// Flex sets the flex factor of the content in a stack.
func (v ContextMenuView) Flex(f float32) ContextMenuView { v.flex = f; return v }

// CornerRadius tells the context menu the corner radius of its content, so
// that the window dims around the content's own shape after a long press.
func (v ContextMenuView) CornerRadius(r float32) ContextMenuView { v.radius = max(0, r); return v }

// ViewType implements gift.View.
func (ContextMenuView) ViewType() gift.TypeID { return contextMenuType }

// Build implements gift.View.
func (v ContextMenuView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.contextMenu", v.body)}, Flex: v.flex}
}

// contextMode is how an open context menu was asked for.
type contextMode struct {
	atPoint bool
	point   geom.Point
}

func (v ContextMenuView) body(ctx *gift.Context) gift.View {
	st, open := menuStateOf(ctx)
	st.setItems(v.items)
	mode := ctx.State("mode", contextMode{})
	ctx.Read(mode)
	return contextHost{content: v.content, st: st, open: open.Get(), mode: mode.Get(), setMode: mode.Set, radius: v.radius}
}

type contextHost struct {
	content gift.View
	st      *menuState
	open    bool
	mode    contextMode
	setMode func(contextMode)
	radius  float32
}

func (contextHost) ViewType() gift.TypeID { return contextHostType }

func (h contextHost) Build(*gift.BuildContext) gift.Element {
	n := &contextNode{h: h}
	e := gift.Element{
		Key:        "ui.contextHost",
		Layouter:   anchorLayout{},
		Interactor: n,
		Children:   []gift.View{h.content},
	}
	if h.open {
		e.Overlay = menuPanel(h.st, !h.mode.atPoint, h.mode.point, h.mode.atPoint, h.radius)
		e.OverlayExit = panelExit
	}
	return e
}

type contextNode struct{ h contextHost }

func (n *contextNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := n.h.st
	switch e.Kind {
	case gift.EventContextMenu:
		st.pressed = false
		n.h.setMode(contextMode{atPoint: true, point: e.Pos})
		st.open()
		return true
	case gift.EventLongPress:
		if n.h.open {
			return false
		}
		ctx.StealPointer()
		st.pressed, st.dragged, st.downPos = true, false, e.Pos
		n.h.setMode(contextMode{})
		st.open()
		return true
	case gift.EventPointerMove:
		if st.pressed {
			st.track(e.Pos)
			ctx.Repaint()
			return true
		}
	case gift.EventPointerUp:
		if st.pressed {
			st.release(e.Pos)
			return true
		}
	case gift.EventPointerCancel:
		st.pressed = false
	}
	return false
}
