package main

import (
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

// App is the application the theme and the keyboard switches rebuild. main
// and the tests set it.
var App *gift.App

// compactBelow is the window width under which the kitchen sink shows the
// list and a component one after the other, in a navigation stack, instead of
// side by side: a five inch panel at 800 by 480 is compact, a desktop window
// is not.
const compactBelow = 900

// sidebarWidth is the width of the list beside a component.
const sidebarWidth = float32(300)

// Screen is the whole kitchen sink: every component of gift behind a list
// grouped like the documentation, each shown with the demo of its page.
//
// On a wide window the list stands beside the component; on a narrow one it
// is the root of a navigation stack and a component is pushed onto it. Which
// one is a fact about the window that only layout knows, so the layouter
// around the panes measures it and reports the change, which rebuilds once
// when the window crosses compactBelow and never otherwise; see [split].
func Screen(ctx *gift.Context) gift.View {
	selected := ctx.State("selected", initialDemo)
	open := ctx.State("open", false)
	compact := ctx.State("compact", false)
	query := ctx.State("query", "")
	ctx.Read(selected)
	ctx.Read(open)
	ctx.Read(compact)
	ctx.Read(query)

	d, ok := lookup(selected.Get())
	if !ok {
		d = pages()[0]
	}
	choose := func(name string) {
		selected.Set(name)
		open.Set(true)
	}
	list := sidebar(ctx, d.Name, compact.Get(), query, choose)

	var panes []gift.View
	if compact.Get() {
		screens := []ui.ScreenSpec{ui.Screen("Components", list).Key("components")}
		if open.Get() {
			screens = append(screens, ui.Screen(d.Title, detail(d)).Key("demo-"+d.Name))
		}
		panes = []gift.View{ui.NavigationStack(func() { open.Set(false) }, screens...)}
	} else {
		panes = []gift.View{list.Background(ui.ColorSurface), detail(d)}
	}
	kids := []gift.View{split{panes: panes, compact: compact.Get(), set: compact.Set}}
	// The keyboard of a kiosk, for the text fields of every demo but the
	// keyboard's own, which brings one.
	if d.Name != "keyboard" || (compact.Get() && !open.Get()) {
		kids = append(kids, ui.OnScreenKeyboard())
	}
	return ui.Window(kids...).Align(geom.Bottom)
}

// initialDemo is the component shown first; main sets it from -show.
var initialDemo = "button"

// page is an entry of the list: a demo of internal/showcase, or one of the
// kitchen sink's own, see [own].
type page struct {
	showcase.Demo
	build  func(ctx *gift.Context) gift.View // nil for a demo of internal/showcase
	source string
}

// View builds the page.
func (p page) View(ctx *gift.Context) gift.View {
	if p.build != nil {
		return p.build(ctx)
	}
	return p.Demo.View(ctx)
}

// pages are the entries of the list: every demo of internal/showcase, then
// the kitchen sink's own.
func pages() []page {
	out := make([]page, 0, len(showcase.All())+len(own))
	for _, d := range showcase.All() {
		out = append(out, page{Demo: d, source: "internal/showcase/" + strings.ReplaceAll(d.Name, "-", "_") + ".go"})
	}
	return append(out, own...)
}

// groups are the sections of the list: those of the documentation, then
// one for what the operating system offers beside the window.
func groups() []showcase.Group {
	return append(showcase.Groups(), showcase.Group{Key: "system", Title: "System"})
}

// lookup returns the page called name.
func lookup(name string) (page, bool) {
	for _, p := range pages() {
		if p.Name == name {
			return p, true
		}
	}
	return page{}, false
}

// sidebar is the header and the list of components, filtered by the query.
func sidebar(ctx *gift.Context, selected string, compact bool, query *gift.State[string], choose func(string)) ui.Stack {
	q := strings.ToLower(strings.TrimSpace(query.Get()))
	var rows []gift.View
	for _, g := range groups() {
		var group []gift.View
		for _, d := range pages() {
			if d.Group != g.Key || !matches(d, q) {
				continue
			}
			name := d.Name
			row := ui.Row(d.Title).OnTap(func() { choose(name) }).Key("row-" + d.Name)
			if compact {
				row = row.Subtitle(d.Summary).Chevron(outline.AngleRight)
			} else {
				row = row.Selected(d.Name == selected)
			}
			group = append(group, row)
		}
		if len(group) > 0 {
			rows = append(rows, ui.Section(g.Title))
			rows = append(rows, group...)
		}
	}
	if len(rows) == 0 {
		rows = append(rows, ui.Text("No component matches.").Foreground(ui.ColorSecondaryLabel).Padding(16))
	}

	// The theme in force, whoever set it: a theme switch rebuilds the whole
	// window, this function included.
	dark := ui.CurrentTheme().IsDark()
	themeIcon := outline.Moon
	if dark {
		themeIcon = outline.Sun
	}
	keyboardTint := ui.Color(ui.ColorSecondaryLabel)
	if ui.OnScreenKeyboardEnabled() {
		keyboardTint = ui.ColorAccent
	}
	iconButton := func(sym ui.Symbol, tint ui.Color, label, key string, fn func()) gift.View {
		return ui.Button(ui.Icon(sym).Size(20).Foreground(tint), fn).
			Background(ui.ColorClear).Border(ui.Border{}).Padding(8).Label(label).Key(key)
	}
	header := ui.HStack(
		ui.VStack(
			ui.Text("gift").FontSize(26),
			ui.Text(itoa(len(showcase.All()))+" components").FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).Gap(2).Align(geom.Alignment{X: 0, Y: 0}).Flex(1),
		iconButton(outline.Keyboard, keyboardTint, "On-screen keyboard", "keyboard-switch", func() {
			ui.SetOnScreenKeyboard(App, !ui.OnScreenKeyboardEnabled())
		}),
		iconButton(themeIcon, ui.ColorLabel, "Dark appearance", "theme-switch", func() {
			if dark {
				ui.SetTheme(App, ui.LightTheme())
			} else {
				ui.SetTheme(App, ui.DarkTheme())
			}
		}),
	).Align(geom.Center)

	return ui.VStack(
		ui.VStack(
			header,
			ui.SearchField(ui.Editor(ctx, "query-editor", query.Get())).Placeholder("Search components").
				Bind(query.Binding()).Key("search"),
		).Gap(12).PaddingInsets(geom.Insets{Left: 16, Right: 16, Top: 16, Bottom: 8}),
		// Beside a component the list is a sidebar, and a sidebar draws no
		// hairlines: the selection sets the page apart, inset from the edges
		// by the padding, as on macOS and iPadOS. Their space stays, a
		// point between two faces, so that the row under the pointer does
		// not merge with the selected one next to it. As the root of a
		// navigation stack it is a plain list, rows with subtitles and
		// hairlines between.
		ui.VScroll(sidebarList(ui.List(rows...), compact)).
			PaddingInsets(geom.Insets{Left: 8, Right: 8, Bottom: 16}).Flex(1).Key("list"),
	).Gap(0)
}

// sidebarList is l without visible hairlines beside a component; see
// [sidebar].
func sidebarList(l ui.ListView, compact bool) ui.ListView {
	if compact {
		return l
	}
	return l.SeparatorColor(ui.ColorClear)
}

func matches(d page, q string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(d.Title), q) || strings.Contains(strings.ToLower(d.Summary), q) ||
		strings.Contains(d.Name, q)
}

// detail is one component: where it belongs, its name and what it does,
// and its demo on a stage of the size the demo was made for, scrolling where
// the window is smaller.
func detail(d page) ui.ScrollView {
	group := ""
	for _, g := range groups() {
		if g.Key == d.Group {
			group = g.Title
		}
	}
	// The demo is a component of its own, keyed by its name: leaving it
	// unmounts it with its state, which closes what it opened, and coming
	// back starts it afresh.
	// The stage is a ZStack with a frame, which measures the demo bounded on
	// both axes: a full window demo is a screen, a navigation stack or a
	// modal say, which fills what it is given and has nothing to fill under
	// a stack that leaves its main axis open.
	demo := gift.Component("demo-"+d.Name, d.View)
	stage := ui.ZStack(demo)
	if !d.FullWindow {
		stage = ui.ZStack(ui.VStack(demo).Padding(24)).Align(geom.Center)
	}
	stage = stage.
		Frame(float32(d.Width), float32(d.Height)).
		Background(ui.ColorBackground).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		CornerRadius(14).
		Clip(true).
		Key("stage-" + d.Name)
	return ui.VScroll(
		ui.Text(strings.ToUpper(group)).FontSize(12).Foreground(ui.ColorSecondaryLabel),
		ui.Text(d.Title).FontSize(30).Key("title"),
		ui.Text(d.Summary).Foreground(ui.ColorSecondaryLabel),
		ui.HScroll(stage).Key("stage-scroll"),
		ui.Text("Source: "+d.source).
			FontSize(12).Foreground(ui.ColorSecondaryLabel),
	).Gap(8).Padding(24).Align(geom.Alignment{X: 0, Y: 0}).Key("detail")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// split lays out the panes of [Screen]: the list at sidebarWidth beside the
// component, both the full height of the window, or the navigation stack of
// a compact window over all of it. It is also what notices the width: when it
// crosses compactBelow, the layout arms a timer of the node, and the timer
// event, which may write state where a layout may not, switches Screen.
type split struct {
	panes   []gift.View
	compact bool
	set     func(bool)
}

var splitType = gift.RegisterType("kitchensink.split")

func (split) ViewType() gift.TypeID { return splitType }

func (s split) Build(*gift.BuildContext) gift.Element {
	n := &splitNode{s: s}
	return gift.Element{Key: "split", Layouter: n, Interactor: n, Children: s.panes, Flex: 1}
}

type splitNode struct {
	s    split
	want bool
}

func (n *splitNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	size := c.Constrain(c.Max)
	if n.want = size.W < compactBelow; n.want != n.s.compact {
		ctx.After(time.Millisecond)
	}
	if ctx.ChildCount() == 1 {
		ctx.Measure(0, geom.Tight(size))
		ctx.Place(0, geom.Point{})
		return size
	}
	side := min(sidebarWidth, size.W)
	ctx.Measure(0, geom.Tight(geom.Sz(side, size.H)))
	ctx.Place(0, geom.Point{})
	ctx.Measure(1, geom.Tight(geom.Sz(size.W-side, size.H)))
	ctx.Place(1, geom.Pt(side, 0))
	return size
}

// HandleEvent switches between the two layouts on the timer of [Layout]; it
// takes nothing else.
func (n *splitNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind == gift.EventTimer && n.want != n.s.compact {
		n.s.set(n.want)
		return true
	}
	return false
}
