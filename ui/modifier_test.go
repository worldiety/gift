package ui_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// This is the check the project plan, section 4, decided on in place of a
// generator: "ein Test, der ueber alle exportierten View-Typen reflektiert und
// den gemeinsamen Modifier-Satz mit korrekter Signatur einfordert."
//
// The failure it catches is real and silent in the small: a new view type that
// is missing Padding compiles perfectly, and the omission only shows up when
// somebody tries to use it, at which point the fix is one line in a file they
// did not expect to touch. A generator would have prevented the same failure
// at the cost of a build step and a second place to look for
// "func (t TextView) Padding".

// modifierSet is the group of modifiers a view type belongs to.
type modifierSet uint8

const (
	// setMinimal is what every view has, no matter what it is.
	setMinimal modifierSet = iota
	// setStyled adds the size and box style modifiers of a view that has
	// bounds it can fill, stroke and clip.
	setStyled
	// setInternal is a view type that exists only as a part of a composite
	// and that an application never constructs. It has no public modifier
	// contract to check, because it has no public constructor; it is listed
	// so that TestEveryViewTypeIsListed stays a check rather than a list
	// with an escape hatch.
	setInternal
)

// views lists one value of every exported view type together with the set it
// promises to implement.
//
// Adding a view type to this package without adding it here fails
// TestEveryViewTypeIsListed below, which is what makes this table a check
// rather than a list somebody forgets.
var views = []struct {
	name string // the name it registered under
	set  modifierSet
	v    any
	// why, for a type that is deliberately not fully styled.
	why string
}{
	{name: "ui.VStack", set: setStyled, v: ui.VStack()},
	{name: "ui.HStack", set: setStyled, v: ui.HStack()},
	{name: "ui.ZStack", set: setStyled, v: ui.ZStack()},
	{name: "ui.Box", set: setStyled, v: ui.Box()},
	{name: "ui.Text", set: setStyled, v: ui.Text("x")},
	{name: "ui.Button", set: setStyled, v: ui.Button(ui.Box(), nil)},
	{name: "ui.VScroll", set: setStyled, v: ui.VScroll()},
	{name: "ui.HScroll", set: setStyled, v: ui.HScroll()},
	{name: "ui.Image", set: setStyled, v: ui.Image(asset.File("x.jpg"))},
	{name: "ui.Icon", set: setStyled, v: ui.Icon(ui.Symbol{})},
	{name: "ui.TextField", set: setStyled, v: ui.TextField(ui.NewTextEditor(""))},
	{name: "ui.TextArea", set: setStyled, v: ui.TextArea(ui.NewTextEditor(""))},
	{name: "ui.CodeEditor", set: setStyled, v: ui.CodeEditor(ui.NewTextEditor(""))},
	{name: "ui.navSweep", set: setInternal, why: "it takes a screen a pop took away once it has slid out; a part of ui.NavigationStack"},
	{name: "ui.TimeField", set: setStyled, v: ui.TimeField()},
	{name: "ui.DateField", set: setStyled, v: ui.DateField()},
	{name: "ui.ImageGallery", set: setStyled, v: ui.ImageGallery(ui.NewGallery(asset.NewCollection(nil)))},
	{name: "ui.OnScreenKeyboard", set: setStyled, v: ui.OnScreenKeyboard()},
	{
		name: "ui.GalleryTile", set: setInternal,
		why: "a tile is one slot of the gallery's recycled pool. It has no exported " +
			"constructor and no styling of its own: what it looks like is ui.TileStyle, " +
			"set on the gallery, because a tile is bound to an item during layout and a " +
			"per tile modifier would have nowhere to be written.",
	},
	{
		name: "ui.Toggle", set: setMinimal, v: ui.Toggle(false, nil),
		why: controlWhy,
	},
	{
		name: "ui.Slider", set: setMinimal, v: ui.Slider(0, nil),
		why: controlWhy,
	},
	{
		name: "ui.SegmentedControl", set: setMinimal, v: ui.SegmentedControl(0, []string{"a"}, nil),
		why: controlWhy,
	},
	{
		name: "ui.ProgressBar", set: setMinimal, v: ui.ProgressBar(0),
		why: controlWhy,
	},
	{
		name: "ui.TabBar", set: setMinimal, v: ui.TabBar(0, nil, ui.Tab("a", ui.Symbol{}, ui.Box())),
		why: navigationWhy,
	},
	{
		name: "ui.NavigationStack", set: setMinimal, v: ui.NavigationStack(nil, ui.Screen("a", ui.Box())),
		why: navigationWhy,
	},
	{
		name: "ui.Modal", set: setMinimal, v: ui.Modal(ui.Box(), nil),
		why: navigationWhy,
	},
	{
		name: "ui.Sheet", set: setMinimal, v: ui.Sheet(ui.Box(), nil),
		why: navigationWhy,
	},
	{
		name: "ui.Popover", set: setMinimal, v: ui.Popover(ui.Box(), nil),
		why: presenterWhy,
	},
	{
		name: "ui.Menu", set: setMinimal, v: ui.Menu(ui.Box()),
		why: presenterWhy,
	},
	{
		name: "ui.ContextMenu", set: setMinimal, v: ui.ContextMenu(ui.Box()),
		why: presenterWhy,
	},
	{
		name: "ui.DropDestination", set: setMinimal, v: ui.DropDestination(ui.Box()),
		why: "a wrapper that takes drops for its content; size and style belong to the content",
	},
	{
		name: "ui.Stepper", set: setMinimal, v: ui.Stepper(0, nil),
		why: "a fixed 94 by 32 pair of buttons, as UIStepper; it has no box of its own to size or style",
	},
	{
		name: "ui.SearchField", set: setMinimal, v: ui.SearchField(ui.NewTextEditor("")),
		why: "the search field of iOS: a TextField in a pill of fixed height and look; a TextField styles freely",
	},
	{name: "ui.searchBox", set: setInternal, why: "the field, the glass and the clear button of a ui.SearchField"},
	{name: "ui.searchClear", set: setInternal, why: "the clear button of a ui.SearchField"},
	{
		name: "ui.PageControl", set: setMinimal, v: ui.PageControl(3, 0, nil),
		why: "a row of dots as wide as the pages need, as UIPageControl; Capsule is its one background",
	},
	{
		name: "ui.Toast", set: setMinimal, v: ui.Toast(ui.Box(), "x"),
		why: presenterWhy,
	},
	{name: "ui.toastLayer", set: setInternal, why: "the window-sized, input-transparent layer a ui.Toast hangs in"},
	{name: "ui.toastCard", set: setInternal, why: "the capsule of a ui.Toast; its settings are ToastView's"},
	{
		name: "ui.Swipe", set: setMinimal, v: ui.Swipe(ui.Box()),
		why: "a row as tall and as wide as its content, which carries the styling; Background is the cover over the actions",
	},
	{name: "ui.swipeRow", set: setInternal, why: "the row of a ui.Swipe with its actions; its settings are SwipeView's"},
	{name: "ui.swipeButton", set: setInternal, why: "one action behind a ui.Swipe row; its settings are SwipeActionView's"},
	{name: "ui.swipeCatcher", set: setInternal, why: "the top layer of an open ui.Swipe row, which closes it on a press elsewhere"},
	{name: "ui.swipeShield", set: setInternal, why: "one rectangle of the catcher of an open ui.Swipe row"},
	{
		name: "ui.ProgressRing", set: setMinimal, v: ui.ProgressRing(0),
		why: "a ring of one size, set by Size and LineWidth; it has no box to style",
	},
	{
		name: "ui.Gauge", set: setMinimal, v: ui.Gauge(0, 0, 1),
		why: "a ring of one size, set by Size and LineWidth, with its labels inside; it has no box to style",
	},
	{
		name: "ui.CircularSlider", set: setMinimal, v: ui.CircularSlider(0, nil),
		why: "a ring with a knob, sized by Size and LineWidth like the Slider it is the round form of",
	},
	{
		name: "ui.ColorPicker", set: setMinimal, v: ui.ColorPicker(ui.RGB(0, 0, 0), nil),
		why: "the panel of UIColorPickerViewController, 320 wide; Background is the colour it stands on",
	},
	{
		name: "ui.ColorWell", set: setMinimal, v: ui.ColorWell(ui.RGB(0, 0, 0), nil),
		why: "a swatch of one size in a ring of hues, as SwiftUI's ColorPicker",
	},
	{name: "ui.colorGrid", set: setInternal, why: "the swatches of a ui.ColorPicker"},
	{name: "ui.colorSpectrum", set: setInternal, why: "the spectrum of a ui.ColorPicker"},
	{name: "ui.colorSlider", set: setInternal, why: "a channel slider of a ui.ColorPicker"},
	{name: "ui.colorSwatch", set: setInternal, why: "the colour shown by a ui.ColorPicker"},
	{name: "ui.colorWellButton", set: setInternal, why: "the swatch of a ui.ColorWell"},
	{
		name: "ui.WheelPicker", set: setMinimal, v: ui.WheelPicker(),
		why: "a wheel has the height of its rows and draws its own band and fading; its size is VisibleRows, RowHeight and Width",
	},
	{
		name: "ui.DateWheel", set: setMinimal, v: ui.DateWheel(),
		why: "three wheels of fixed widths, styled like a ui.WheelPicker",
	},
	{
		name: "ui.TimeWheel", set: setMinimal, v: ui.TimeWheel(),
		why: "two wheels of fixed widths, styled like a ui.WheelPicker",
	},
	{
		name: "ui.Calendar", set: setMinimal, v: ui.Calendar(),
		why: "a month grid of fixed rows, styled like the inline date picker of iOS; its size is Width",
	},
	{name: "ui.calendarCore", set: setInternal, why: "the header, weekday row and pager of a ui.Calendar, which carries its settings"},
	{name: "ui.calendarMonth", set: setInternal, why: "the days of one month of a ui.Calendar"},
	{name: "ui.calendarArrow", set: setInternal, why: "the previous and next month buttons of a ui.Calendar"},
	{name: "ui.wheelColumn", set: setInternal, why: "one wheel of a ui.WheelPicker; its settings are WheelColumn's"},
	{name: "ui.panel", set: setInternal, why: "the card of a popover and of a menu in the top layer; its settings are PopoverView's"},
	{name: "ui.menuButton", set: setInternal, why: "the control of ui.Menu, which carries its settings"},
	{name: "ui.menuList", set: setInternal, why: "the rows of an open menu"},
	{name: "ui.menuRow", set: setInternal, why: "one item of an open menu; its settings are MenuItemView's"},
	{name: "ui.menuRule", set: setInternal, why: "the hairline or the band between items of an open menu"},
	{name: "ui.contextHost", set: setInternal, why: "the content of ui.ContextMenu with its input"},
	{
		name: "ui.sheetPanel", set: setInternal,
		why: "the node a presented sheet hangs under: it sizes the panel from the window " +
			"and is the thing that moves. Its settings are SheetView.Height and " +
			"SheetView.MaxWidth. See ui/sheet.go.",
	},
	{
		name: "ui.Alert", set: setMinimal, v: ui.Alert("a", "b"),
		why: "an alert is a card with a fixed width, a fixed radius and the surface " +
			"colour, because those three are what makes it recognisable as an alert " +
			"rather than as a panel. A caller who wants a differently shaped card " +
			"hands that card to ui.Modal directly; the presentation takes any view.",
	},
	{
		name: "ui.layer", set: setInternal,
		why: "the layer is one screen of a TabBar, a NavigationStack or a Modal. It has " +
			"no exported constructor: what it carries is gift.Element.Hidden and " +
			"gift.Element.FocusTrap, and neither is a knob an application should be " +
			"able to put on an arbitrary view. See ui/layer.go.",
	},
	{
		name: "ui.scrim", set: setInternal,
		why: "the scrim is the input barrier ui.Modal puts between the content and the " +
			"modal. It has no exported constructor and its only two settings, the wash " +
			"colour and the dismiss action, are ModalView.Scrim and ModalView.OnDismiss.",
	},
	{
		name: "ui.modalDialog", set: setInternal,
		why: "the node a presented modal hangs under, and the thing that moves when one " +
			"opens or closes. It is not a layer because a layer fills the window and " +
			"would stretch an alert card across it; it carries gift.Element.Hidden and " +
			"gift.Element.Transition and nothing else. See ui/modal.go.",
	},
	{
		name: "ui.Spacer", set: setMinimal, v: ui.Spacer(),
		why: "a Spacer draws nothing and has no bounds of its own; a Background it then " +
			"ignored would be exactly the lie variant A exists to avoid. See SpacerView.",
	},
	{name: "ui.List", set: setStyled, v: ui.List()},
	{name: "ui.VirtualList", set: setStyled, v: ui.VirtualList(ui.NewListState(), 0, nil)},
	{
		name: "ui.VirtualListRow", set: setInternal,
		why: "a row is one slot of the virtual list's recycled pool, bound to an item " +
			"during layout. What it shows is ui.ListRow, returned by the row function, " +
			"because a per row modifier would have nowhere to be written.",
	},
	{
		name: "ui.Row", set: setMinimal, v: ui.Row("x"),
		why: "a row is the standard row of a ui.List, and its height, its horizontal inset " +
			"and its face in each interaction state are the convention that makes a list " +
			"look like one list. A Padding on a row would move its content out from under " +
			"the separator that is inset to match it, and a Background would be a face " +
			"that the pressed state then replaces. A differently shaped row is an " +
			"ordinary HStack handed to ui.List, which takes any view at all.",
	},
	{
		name: "ui.DisclosureGroup", set: setMinimal, v: ui.DisclosureGroup("x"),
		why: "its label is a ui.Row and takes a row's options, for the reason ui.Row has no " +
			"box modifiers; the content is the caller's views, styled as the caller likes.",
	},
	{
		name: "ui.Accordion", set: setMinimal, v: ui.Accordion(),
		why: "an accordion is a ui.List of disclosure groups; it forwards the list's Frame " +
			"and separators, and a styled box around it is a ui.Card.",
	},
	{name: "ui.disclosureBody", set: setInternal, why: "the label and the content of a ui.DisclosureGroup, which carries its settings"},
	{name: "ui.disclosureContent", set: setInternal, why: "the clip that reveals the content of a ui.DisclosureGroup"},
	{name: "ui.disclosureChevron", set: setInternal, why: "the turning chevron of a ui.DisclosureGroup"},
	{
		name: "ui.Section", set: setMinimal, v: ui.Section("x"),
		why: "a section header is one caption with the inset and the type size that make " +
			"it read as a header; it is also the value ui.List recognises when it decides " +
			"where a separator goes, so a header that could be restyled into something " +
			"else would be a header that no longer marks a boundary. See ui.ListView.",
	},
	{
		name: "ui.Divider", set: setMinimal, v: ui.Divider(),
		why: "a divider is a hairline. Its three properties are its thickness, its colour " +
			"and how far it is inset, and each has a modifier of its own; the box " +
			"modifiers have nothing to apply to, because the line *is* the node. A " +
			"Padding would be indistinguishable from an Inset and a Background from a " +
			"Color.",
	},
	{
		name: "ui.Badge", set: setMinimal, v: ui.Badge("1"),
		why: "a badge is a capsule of a fixed height, which is what makes a column of " +
			"badges line up and what makes its corner radius exactly half its height. A " +
			"Frame or a CornerRadius would break one or the other, and the two things a " +
			"design does want to change, the fill and the text colour, are Color and " +
			"Foreground.",
	},
	{
		name: "ui.Card", set: setMinimal, v: ui.Card(),
		why: "a card is the themed face, the corner radius and the hairline that make a " +
			"panel recognisable as raised above the window; a card whose background and " +
			"radius were modifiers would be a ui.VStack with extra steps. What is a " +
			"decision of the call site — how far the content is inset and how far apart " +
			"the children sit — is Padding, PaddingInsets and Gap, which it has.",
	},
}

// minimalModifiers are required of every view type. The signature is given as
// the argument types; the result is always the receiver's own concrete type,
// which is the property variant A rests on and which is checked separately.
// navigationWhy is why the three navigation containers carry the minimal set
// only.
//
// They are not boxes. A TabBar and a NavigationStack *are* the window: their
// geometry is "fill whatever you are given", and the bar, the hairline and the
// surface behind them are what makes each of them recognisable as the thing it
// is rather than as a stack somebody styled. A Padding on a tab bar would put
// the bar's own background inside the padding and leave the window showing
// through around it, and a Frame on one would be a tab bar that does not reach
// the bottom of the screen — which is not a layout an application wants and is
// two lines of VStack away for one that does. A Modal is transparent: it has
// no bounds of its own beyond the ones its content takes.
// presenterWhy is why the views that present a popover or a menu carry the
// minimal set only.
const presenterWhy = "it takes the size of the view it wraps, which carries its own modifiers, " +
	"and what it presents is drawn in the top layer, styled by its own settings"

const navigationWhy = "a navigation container fills what it is given and draws its own chrome; " +
	"see ui.TabBarView on why its metrics are constants rather than modifiers"

var minimalModifiers = []struct {
	name string
	args []reflect.Type
}{
	{"Key", []reflect.Type{stringType}},
	{"Flex", []reflect.Type{float32Type}},
}

// styledModifiers are required in addition of every view with bounds.
var styledModifiers = []struct {
	name string
	args []reflect.Type
}{
	{"Padding", []reflect.Type{float32Type}},
	{"PaddingInsets", []reflect.Type{insetsType}},
	{"Frame", []reflect.Type{float32Type, float32Type}},
	{"MinWidth", []reflect.Type{float32Type}},
	{"MinHeight", []reflect.Type{float32Type}},
	{"MaxWidth", []reflect.Type{float32Type}},
	{"MaxHeight", []reflect.Type{float32Type}},
	{"Background", []reflect.Type{backgroundType}},
	{"Border", []reflect.Type{borderType}},
	{"Shadow", []reflect.Type{shadowType}},
	{"CornerRadius", []reflect.Type{float32Type}},
	{"Clip", []reflect.Type{boolType}},
}

var (
	stringType  = reflect.TypeOf("")
	float32Type = reflect.TypeOf(float32(0))
	boolType    = reflect.TypeOf(false)
	insetsType  = reflect.TypeOf(geom.Insets{})
	colorType   = reflect.TypeOf(ui.Color{})
	// Background takes an interface and not a Color, because the project
	// plan, section 8, spells a material as
	// ".Background(ui.Glass().Quality(ui.Adaptive))" and Go has no
	// overloading. ui.Color still satisfies it, so ".Background(ui.RGB(...))"
	// is unchanged; TestBackgroundAcceptsBothKinds pins that.
	backgroundType = reflect.TypeOf((*ui.Background)(nil)).Elem()
	borderType     = reflect.TypeOf(ui.Border{})
	shadowType     = reflect.TypeOf(ui.Shadow{})
)

func TestViewsCarryTheSharedModifierSet(t *testing.T) {
	for _, view := range views {
		if view.set == setInternal {
			continue
		}
		rt := reflect.TypeOf(view.v)
		want := minimalModifiers
		if view.set == setStyled {
			want = append(append([]struct {
				name string
				args []reflect.Type
			}{}, minimalModifiers...), styledModifiers...)
		}
		for _, mod := range want {
			m, ok := rt.MethodByName(mod.name)
			if !ok {
				t.Errorf("%s has no method %s; every view with bounds carries the shared modifier set, "+
					"see the project plan, section 4", rt, mod.name)
				continue
			}
			ft := m.Type
			// Method obtained from a type, so argument 0 is the receiver.
			if got, want := ft.NumIn()-1, len(mod.args); got != want {
				t.Errorf("%s.%s takes %d arguments, want %d", rt, mod.name, got, want)
				continue
			}
			for i, at := range mod.args {
				if got := ft.In(i + 1); got != at {
					t.Errorf("%s.%s argument %d is %v, want %v", rt, mod.name, i, got, at)
				}
			}
			if ft.NumOut() != 1 || ft.Out(0) != rt {
				t.Errorf("%s.%s returns %v, want its own type %v; a modifier that returned an "+
					"interface would end the fluent chain and is what variant A rejects", rt, mod.name, outOf(ft), rt)
			}
		}
	}
}

func outOf(ft reflect.Type) string {
	if ft.NumOut() == 0 {
		return "nothing"
	}
	return ft.Out(0).String()
}

// TestEveryViewTypeIsListed is the half that makes the test above fail for a
// view type that does not exist yet.
//
// Reflection cannot enumerate the types of a package, so the list of view
// types is taken from gift's type registry instead: a view type that is not
// registered cannot be reconciled, so every one of them is in there.
func TestEveryViewTypeIsListed(t *testing.T) {
	registered := gift.RegisteredTypeNames(nil)
	for _, name := range registered {
		if !strings.HasPrefix(name, "ui.") {
			continue
		}
		found := false
		for _, v := range views {
			if v.name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the view type %q is registered but not listed in views; add it there and "+
				"say which modifier set it implements", name)
		}
	}
	for _, v := range views {
		found := false
		for _, name := range registered {
			if name == v.name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("views lists %q, which no longer registers itself under that name", v.name)
		}
	}
}

// TestEveryViewIsAView keeps the table honest about what it contains.
func TestEveryViewIsAView(t *testing.T) {
	for _, v := range views {
		if v.set == setInternal {
			continue
		}
		if _, ok := v.v.(gift.View); !ok {
			t.Errorf("%T is in views but does not implement gift.View", v.v)
		}
	}
}

// controlWhy is the reason the four selection and indication controls carry
// the minimal modifier set rather than the styled one.
//
// It is one string because the argument is one argument, and repeating it four
// times in slightly different words is how four widgets drift apart.
const controlWhy = "a Toggle, a Slider, a SegmentedControl and a ProgressBar are not boxes with " +
	"content in them: their whole appearance is the control, and a Background, a Border or a " +
	"CornerRadius behind it would be a second, invisible way of describing a shape the widget " +
	"already owns — a switch with a square corner radius is not a switch. What a caller may " +
	"change is the size of the hit area, with .Frame, and, on the three that have an accent " +
	"coloured part, that accent with .Tint. SegmentedControl deliberately has no .Tint: its " +
	"indicator is ColorSurface and not ColorAccent on purpose, because a view switch whose " +
	"current tab wore the accent would compete with the accent used for the actions on the " +
	"screen it selects, so there is no accent in it to tint. The rest is the theme's. See " +
	"ui.ControlHitTarget and ui.SegmentedControlView."

// TestOnlyTheControlsThatHaveAnAccentCarryATint is the test [controlWhy] did
// not have, and its absence is why that string spent a work unit telling
// callers to reach for a modifier one of the four widgets it names does not
// have.
//
// The asymmetry is deliberate and is argued in [ui.SegmentedControlView]: the
// sliding indicator is ColorSurface and not ColorAccent, because a view switch
// whose current tab wore the accent would compete with the accent used for the
// actions on the screen it selects. There is therefore no accent in a
// segmented control for a Tint to change, and adding one would invite exactly
// the design that documentation argues against.
func TestOnlyTheControlsThatHaveAnAccentCarryATint(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want bool
	}{
		{ui.Toggle(false, nil), true},
		{ui.Slider(0, nil), true},
		{ui.ProgressBar(0), true},
		{ui.SegmentedControl(0, []string{"a"}, nil), false},
	} {
		typ := reflect.TypeOf(tc.v)
		_, got := typ.MethodByName("Tint")
		if got == tc.want {
			continue
		}
		if tc.want {
			t.Errorf("%s has no Tint, but controlWhy tells callers to change its accent with "+
				"one. Add the modifier or rewrite that string", typ.Name())
			continue
		}
		t.Errorf("%s has grown a Tint. controlWhy says it deliberately has none, and "+
			"SegmentedControlView argues at length why its indicator must not be the accent; "+
			"if that argument has been overturned, overturn it in the prose too", typ.Name())
	}
}

// TestACardWithAHeaderPassesItsFlexOn: a scroll container in a flexed card
// with a header gets the card's height. The header wrapper used to measure the
// content stack as an inflexible child, with an unbounded main axis, so the
// scroll container's Flex had nothing to divide and it came out zero tall.
func TestACardWithAHeaderPassesItsFlexOn(t *testing.T) {
	for _, header := range []string{"", "Log"} {
		lines := make([]gift.View, 40)
		for i := range lines {
			lines[i] = ui.Text("line").Key(strconv.Itoa(i))
		}
		h := gifttest.New(t, gifttest.Options{
			Font: loadTestFont(t), Size: geom.Sz(300, 400),
			View: ui.VStack(ui.Card(ui.VScroll(lines...).Flex(1).Key("log")).Header(header).Flex(1)).Flex(1),
		})
		h.Settle()
		if b := h.Find(gifttest.ByKey("log")).Bounds(); b.Height() < 100 {
			t.Errorf("header %q: the scroll container is %v, want most of the card", header, b)
		}
	}
}

// TestAFlexedCardWithAHeaderInARowHoldsItsContent: a card flexed across an
// [ui.HStack] shares the row's width and keeps its own height. Handing the
// card's Flex to its content stack stretched the card to a bounded row's
// height, and in an unbounded row measured that stack zero tall, so the text
// hung below a card that was only its header.
func TestAFlexedCardWithAHeaderInARowHoldsItsContent(t *testing.T) {
	for _, rowFlex := range []float32{0, 1} {
		h := gifttest.New(t, gifttest.Options{
			Font: loadTestFont(t), Size: geom.Sz(480, 240),
			View: ui.VStack(ui.HStack(
				ui.Card(ui.Text("Delivery").Key("text")).Header("Today").Flex(1).Key("card"),
				ui.Card(ui.Text("Report")).Header("Yesterday").Flex(1),
			).Flex(rowFlex)).Flex(1),
		})
		h.Settle()
		card, text := h.Find(gifttest.ByKey("card")).Bounds(), h.Find(gifttest.ByKey("text")).Bounds()
		if text.Max.Y > card.Max.Y || text.Height() == 0 {
			t.Errorf("row flex %v: the text is %v, outside the card %v", rowFlex, text, card)
		}
		if card.Height() > 150 {
			t.Errorf("row flex %v: the card is %v, stretched to the row instead of shrink-wrapped", rowFlex, card)
		}
	}
}
