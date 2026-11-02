package handoff

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/solid"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/qr"
)

// State is where a flow on the user's phone stands, as the application
// learns it: from the token endpoint for a login, from its backend for a
// payment.
type State uint8

// The states of a flow.
const (
	// Waiting shows the code: the flow runs on the phone, or has not begun.
	Waiting State = iota
	// Done shows a check mark instead of the code: signed in, or paid.
	Done
	// Failed shows that the flow ended without success, declined say, with
	// the message and, given OnRestart, a button to try again.
	Failed
	// Expired shows that the code is no longer valid, with a button for a
	// new one given OnRestart. A view with an expiry enters it by itself.
	Expired
)

func (s State) String() string {
	switch s {
	case Waiting:
		return "Waiting"
	case Done:
		return "Done"
	case Failed:
		return "Failed"
	case Expired:
		return "Expired"
	}
	return fmt.Sprintf("State(%d)", uint8(s))
}

// Texts are the words of the views of this package. A field left empty is
// the English default; the application gives the ones of its language:
//
//	handoff.Device(da).Texts(handoff.Texts{SignInTitle: "Mit dem Handy anmelden", ...})
//
// Expires holds a %s for the time left, as "14:32".
type Texts struct {
	SignInTitle string // Sign in with your phone
	SignInScan  string // Scan the code with the camera of your phone.
	SignInOpen  string // Or open this address on any device:
	SignInCode  string // and enter this code:
	Requesting  string // Getting a code …
	SignedIn    string // Signed in
	Declined    string // The sign-in was declined.

	PayTitle     string // Pay with your phone
	PayScan      string // Scan the code with the camera of your phone and pay there.
	PayWaiting   string // Waiting for the payment …
	PayPreparing string // Preparing the payment …
	Paid         string // Paid. Thank you!
	PayFailed    string // The payment did not go through.

	Expires  string // Valid for %s.
	Expired  string // The code has expired.
	Failed   string // Something went wrong.
	NewCode  string // New code
	TryAgain string // Try again
}

var english = Texts{
	SignInTitle: "Sign in with your phone",
	SignInScan:  "Scan the code with the camera of your phone.",
	SignInOpen:  "Or open this address on any device:",
	SignInCode:  "and enter this code:",
	Requesting:  "Getting a code …",
	SignedIn:    "Signed in",
	Declined:    "The sign-in was declined.",

	PayTitle:     "Pay with your phone",
	PayScan:      "Scan the code with the camera of your phone and pay there.",
	PayWaiting:   "Waiting for the payment …",
	PayPreparing: "Preparing the payment …",
	Paid:         "Paid. Thank you!",
	PayFailed:    "The payment did not go through.",

	Expires:  "Valid for %s.",
	Expired:  "The code has expired.",
	Failed:   "Something went wrong.",
	NewCode:  "New code",
	TryAgain: "Try again",
}

// withDefaults fills the empty fields of t with the English ones.
func (t Texts) withDefaults() Texts {
	r := english
	src, dst := reflect.ValueOf(t), reflect.ValueOf(&r).Elem()
	for i := range src.NumField() {
		if s := src.Field(i).String(); s != "" {
			dst.Field(i).SetString(s)
		}
	}
	return r
}

// green is the colour of a check mark, iOS's system green.
var green = ui.RGB(52, 199, 89)

// picture is what the square of a panel shows: the code while the flow
// waits for the phone, a sign once it is over, a spinner while there is no
// code yet.
func picture(state State, code, label string) gift.View {
	sign := func(s ui.Symbol, c ui.Color) gift.View {
		return ui.ZStack(ui.Icon(s).Size(96).Foreground(c)).Align(geom.Center).
			Background(ui.ColorControl).CornerRadius(16)
	}
	switch state {
	case Done:
		return sign(solid.CheckCircle, green)
	case Failed:
		return sign(solid.ExclamationCircle, ui.ColorDanger)
	case Expired:
		return sign(solid.Clock, ui.ColorSecondaryLabel)
	}
	if code == "" {
		return ui.ZStack(ui.ProgressRing(0).Indeterminate()).Align(geom.Center).
			Background(ui.ColorControl).CornerRadius(16)
	}
	return qr.View(code).Label(label)
}

func title(s string) gift.View {
	return ui.Text(s).Font(bold()).FontSize(26)
}

func line(s string) gift.View {
	return ui.Text(s).FontSize(17).Foreground(ui.ColorSecondaryLabel)
}

func small(s string) gift.View {
	return ui.Text(s).FontSize(15).Foreground(ui.ColorSecondaryLabel)
}

func restartButton(label string, fn func()) gift.View {
	face := ui.ButtonStyle{Background: ui.ColorAccent, CornerRadius: 10}
	pressed := ui.ButtonStyle{Background: ui.Fade(ui.ColorAccent, 0.8), CornerRadius: 10}
	return ui.Button(ui.Text(label).FontSize(17).Foreground(ui.ColorOnAccent), fn).
		Style(face).PressedStyle(pressed).
		PaddingInsets(geom.Insets{Top: 10, Right: 20, Bottom: 10, Left: 20}).
		MinHeight(ui.ControlHitTarget).Key("restart")
}

// info is the column of text beside the picture.
func info(kids ...gift.View) gift.View {
	return ui.VStack(kids...).Gap(10).Align(geom.Alignment{X: 0, Y: 0}).MaxWidth(440)
}

// bold is the bold face of the family of the default font, or the default
// font where it has none.
func bold() ui.Font {
	def := ui.DefaultFont()
	for _, f := range ui.RegisteredFonts(nil) {
		if f.Font == def {
			if b, ok := ui.ResolveFont(ui.FontQuery{Family: f.Family, Weight: ui.WeightBold}); ok {
				return b
			}
		}
	}
	return def
}

// address is a URL as a person types it: without the scheme, and without
// the slash of an empty path.
func address(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(u, "/")
}

// clock formats the time left as a countdown shows it, 14:32 or 1:02:03,
// rounded up to the second.
func clock(d time.Duration) string {
	s := int(math.Ceil(d.Seconds()))
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// countdown returns the time left until expiry, and whether it is over, and
// has the component built again when the countdown shows the next second.
func countdown(ctx *gift.Context, expiry time.Time) (time.Duration, bool) {
	tick := ctx.State("tick", 0)
	ctx.Read(tick)
	if expiry.IsZero() {
		return 0, false
	}
	left := time.Until(expiry)
	if left <= 0 {
		return 0, true
	}
	wait := left % time.Second
	if wait == 0 {
		wait = time.Second
	}
	tok := ctx.Request()
	go func() {
		time.Sleep(wait + 5*time.Millisecond)
		tok.Post(func() { tick.Set(tick.Get() + 1) })
	}()
	return left, false
}

var panelType = gift.RegisterType("handoff.panel")

// panel lays out the picture and the information beside each other where
// there is room for both, and the picture above the information where there
// is not, as on a phone held upright.
type panel struct {
	picture, info gift.View
	key           string
}

func (panel) ViewType() gift.TypeID { return panelType }

func (p panel) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: p.key, Layouter: panelLayout{}, Children: []gift.View{p.picture, p.info}}
}

type panelLayout struct{}

const (
	panelGap     = 32
	minPicture   = 160
	maxPicture   = 320
	minInfoWidth = 260
)

func (panelLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	w, h := c.Max.W, c.Max.H
	if math.IsInf(float64(w), 1) {
		w = 2 * maxPicture
	}
	if math.IsInf(float64(h), 1) {
		h = 2 * maxPicture
	}
	clamp := func(v float32) float32 { return min(max(v, minPicture), maxPicture) }
	if side := clamp(min(w*0.45, h)); w >= side+panelGap+minInfoWidth {
		ctx.Measure(0, geom.Tight(geom.Sz(side, side)))
		in := ctx.Measure(1, geom.Constraints{Max: geom.Sz(w-side-panelGap, h)})
		height := max(side, in.H)
		ctx.Place(0, geom.Pt(0, (height-side)/2))
		ctx.Place(1, geom.Pt(side+panelGap, (height-in.H)/2))
		return c.Constrain(geom.Sz(side+panelGap+in.W, height))
	}
	side := min(clamp(min(w, h*0.5)), w)
	ctx.Measure(0, geom.Tight(geom.Sz(side, side)))
	in := ctx.Measure(1, geom.Constraints{Max: geom.Sz(w, float32(math.Inf(1)))})
	width := max(side, in.W)
	ctx.Place(0, geom.Pt((width-side)/2, 0))
	ctx.Place(1, geom.Pt((width-in.W)/2, side+panelGap))
	return c.Constrain(geom.Sz(width, side+panelGap+in.H))
}

// passLayout lays out the one child of a view that wraps a component.
type passLayout struct{}

func (passLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}
