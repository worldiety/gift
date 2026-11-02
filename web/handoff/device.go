package handoff

import (
	"fmt"

	"golang.org/x/oauth2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

var deviceType = gift.RegisterType("handoff.Device")

// DeviceView shows a device authorization: the code the user enters on
// their phone. It is created by [Device].
type DeviceView struct {
	da      *oauth2.DeviceAuthResponse
	state   State
	msg     string
	restart func()
	texts   Texts
	key     string
}

// Device returns a view of a device authorization of RFC 8628, as
// golang.org/x/oauth2's Config.DeviceAuth returns it: a QR code of the
// verification URI, the address to type instead and the user code, large,
// with the time the code is valid for. Without a complete verification URI,
// which Microsoft Entra does not send, the QR code opens the page where the
// code is typed. A nil da shows the view waiting for a code.
//
// The view shows what it is told; the polling of the token endpoint is the
// application's, or [Login]'s, which does all of it. It enters [Expired] by
// itself when the code expires.
func Device(da *oauth2.DeviceAuthResponse) DeviceView { return DeviceView{da: da} }

// State sets where the login stands, [Waiting] by default.
func (v DeviceView) State(s State) DeviceView { v.state = s; return v }

// Message sets a line in place of the default text of the state: why it
// failed, say.
func (v DeviceView) Message(s string) DeviceView { v.msg = s; return v }

// OnRestart shows a button for a new code once the code has expired or the
// login failed, and calls fn when it is pressed.
func (v DeviceView) OnRestart(fn func()) DeviceView { v.restart = fn; return v }

// Texts sets the words of the view; see [Texts].
func (v DeviceView) Texts(t Texts) DeviceView { v.texts = t; return v }

// Key sets the reconciliation key.
func (v DeviceView) Key(s string) DeviceView { v.key = s; return v }

// ViewType implements gift.View.
func (DeviceView) ViewType() gift.TypeID { return deviceType }

// Build implements gift.View.
func (v DeviceView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: passLayout{},
		Children: []gift.View{gift.Component("handoff.device", v.body)}}
}

func (v DeviceView) body(ctx *gift.Context) gift.View {
	t := v.texts.withDefaults()
	state, da := v.state, v.da
	var expiry string
	if state == Waiting && da != nil {
		left, over := countdown(ctx, da.Expiry)
		if over {
			state = Expired
		} else if left > 0 {
			expiry = fmt.Sprintf(t.Expires, clock(left))
		}
	}
	or := func(s, def string) string {
		if v.msg != "" {
			return v.msg
		}
		return def
	}

	var kids []gift.View
	switch {
	case state == Done:
		kids = append(kids, title(t.SignedIn))
		if v.msg != "" {
			kids = append(kids, line(v.msg))
		}
	case state == Failed || state == Expired:
		msg := or(v.msg, t.Failed)
		label := t.TryAgain
		if state == Expired {
			msg, label = or(v.msg, t.Expired), t.NewCode
		}
		kids = append(kids, title(t.SignInTitle), line(msg))
		if v.restart != nil {
			kids = append(kids, restartButton(label, v.restart))
		}
	case da == nil:
		kids = append(kids, title(t.SignInTitle), line(or(v.msg, t.Requesting)))
	default:
		kids = append(kids, title(t.SignInTitle), line(t.SignInScan), line(t.SignInOpen),
			ui.Text(address(da.VerificationURI)).FontSize(19).Foreground(ui.ColorAccent),
			line(t.SignInCode),
			ui.Text(da.UserCode).Font(bold()).FontSize(34).
				PaddingInsets(geom.Insets{Top: 6, Right: 16, Bottom: 6, Left: 16}).
				Background(ui.ColorControl).CornerRadius(12))
		if v.msg != "" {
			kids = append(kids, small(v.msg))
		}
		if expiry != "" {
			kids = append(kids, small(expiry))
		}
	}

	code := ""
	if da != nil {
		code = da.VerificationURIComplete
		if code == "" {
			code = da.VerificationURI
		}
	}
	return panel{picture: picture(state, code, "QR code to sign in"), info: info(kids...)}
}
