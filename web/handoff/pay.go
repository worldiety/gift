package handoff

import (
	"fmt"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

var payType = gift.RegisterType("handoff.Pay")

// PayView shows a payment the user makes on their phone. It is created by
// [Pay].
type PayView struct {
	url     string
	amount  string
	expiry  time.Time
	state   State
	msg     string
	restart func()
	texts   Texts
	key     string
}

// Pay returns a view of a payment link as a QR code, for a payment the user
// makes on their own phone: a Stripe Payment Link or Checkout Session, a
// PayPal order, any page of a payment provider. The card never touches the
// shared screen, which keeps a kiosk out of the scope of PCI DSS for card
// data, and the phone has the wallets, the bank's app for 3-D Secure and the
// user's own saved cards.
//
// Whether the payment succeeded, the application learns from its backend,
// which learns it from the provider's webhook, and tells the view with
// [PayView.State]. An empty url shows the view waiting for a link.
func Pay(url string) PayView { return PayView{url: url} }

// Amount sets the amount, as the application formats it: "12,50 €".
func (v PayView) Amount(s string) PayView { v.amount = s; return v }

// Expiry sets when the link expires, for a countdown; the view enters
// [Expired] by itself then.
func (v PayView) Expiry(t time.Time) PayView { v.expiry = t; return v }

// State sets where the payment stands, [Waiting] by default.
func (v PayView) State(s State) PayView { v.state = s; return v }

// Message sets a line in place of the default text of the state.
func (v PayView) Message(s string) PayView { v.msg = s; return v }

// OnRestart shows a button to try again once the link has expired or the
// payment failed, and calls fn when it is pressed.
func (v PayView) OnRestart(fn func()) PayView { v.restart = fn; return v }

// Texts sets the words of the view; see [Texts].
func (v PayView) Texts(t Texts) PayView { v.texts = t; return v }

// Key sets the reconciliation key.
func (v PayView) Key(s string) PayView { v.key = s; return v }

// ViewType implements gift.View.
func (PayView) ViewType() gift.TypeID { return payType }

// Build implements gift.View.
func (v PayView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: passLayout{},
		Children: []gift.View{gift.Component("handoff.pay", v.body)}}
}

func (v PayView) body(ctx *gift.Context) gift.View {
	t := v.texts.withDefaults()
	state := v.state
	var expiry string
	if state == Waiting && v.url != "" {
		left, over := countdown(ctx, v.expiry)
		if over {
			state = Expired
		} else if left > 0 {
			expiry = fmt.Sprintf(t.Expires, clock(left))
		}
	}
	msg := func(def string) string {
		if v.msg != "" {
			return v.msg
		}
		return def
	}

	kids := []gift.View{title(t.PayTitle)}
	if state == Done {
		kids[0] = title(t.Paid)
	}
	if v.amount != "" {
		kids = append(kids, ui.Text(v.amount).Font(bold()).FontSize(34))
	}
	switch {
	case state == Done:
		if v.msg != "" {
			kids = append(kids, line(v.msg))
		}
	case state == Failed || state == Expired:
		text, label := msg(t.PayFailed), t.TryAgain
		if state == Expired {
			text = msg(t.Expired)
		}
		kids = append(kids, line(text))
		if v.restart != nil {
			kids = append(kids, restartButton(label, v.restart))
		}
	case v.url == "":
		kids = append(kids, line(msg(t.PayPreparing)))
	default:
		kids = append(kids, line(t.PayScan), small(msg(t.PayWaiting)))
		if expiry != "" {
			kids = append(kids, small(expiry))
		}
	}
	return panel{picture: picture(state, v.url, "QR code to pay"), info: info(kids...)}
}
