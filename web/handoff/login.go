package handoff

import (
	"context"
	"errors"

	"golang.org/x/oauth2"

	"github.com/worldiety/gift"
)

var loginType = gift.RegisterType("handoff.Login")

// LoginView runs a device flow and shows it. It is created by [Login].
type LoginView struct {
	conf   *oauth2.Config
	done   func(*oauth2.Token)
	failed func(error)
	opts   []oauth2.AuthCodeOption
	ctx    context.Context
	texts  Texts
	key    string
}

// Login returns a view that signs the user in on their phone, with the
// device authorization grant of RFC 8628: it asks conf's DeviceAuthURL for
// a code, shows it as [Device] does, polls the token endpoint as
// golang.org/x/oauth2 does, at the interval the server asks for, and calls
// done with the token on the UI goroutine.
//
//	conf := &oauth2.Config{
//		ClientID: "…",
//		Scopes:   []string{"openid", "profile"},
//		Endpoint: oauth2.Endpoint{
//			DeviceAuthURL: "https://idp.example/device/code",
//			TokenURL:      "https://idp.example/token",
//		},
//	}
//	handoff.Login(conf, func(tok *oauth2.Token) { session.Set(tok) })
//
// An expired code and a declined login show a button for a new code. The
// flow ends when the view is taken out of the tree; no code is requested
// before it is shown.
//
// Google, Microsoft Entra, GitHub, Auth0, Okta, Keycloak and Zitadel support
// the device flow, each with its own conditions: GitHub has to have it turned
// on for the application, Google allows only a few scopes with it, and a
// Microsoft Entra tenant may block it by conditional access, as Microsoft
// recommends against phishing. A kiosk that serves such a tenant needs a
// browser on the device instead.
func Login(conf *oauth2.Config, done func(*oauth2.Token)) LoginView {
	return LoginView{conf: conf, done: done}
}

// Options sets parameters of the authorization and token requests, as
// oauth2.SetAuthURLParam makes them: an audience, say.
func (v LoginView) Options(opts ...oauth2.AuthCodeOption) LoginView { v.opts = opts; return v }

// Context sets the context the requests are made with, for an HTTP client
// of the application's own (oauth2.HTTPClient) or to end the flow; the view
// ends it as well when it is taken out of the tree.
func (v LoginView) Context(ctx context.Context) LoginView { v.ctx = ctx; return v }

// OnError calls fn with the errors the view shows as a failure other than a
// declined login: an unreachable server, a client the server does not know,
// a tenant that blocks the device flow. For the log; the view shows them.
func (v LoginView) OnError(fn func(error)) LoginView { v.failed = fn; return v }

// Texts sets the words of the view; see [Texts].
func (v LoginView) Texts(t Texts) LoginView { v.texts = t; return v }

// Key sets the reconciliation key.
func (v LoginView) Key(s string) LoginView { v.key = s; return v }

// ViewType implements gift.View.
func (LoginView) ViewType() gift.TypeID { return loginType }

// Build implements gift.View.
func (v LoginView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Layouter: passLayout{},
		Children: []gift.View{gift.Component("handoff.login", v.body)}}
}

// login is what a LoginView keeps across builds: the run of the flow and
// what it has learned. It is written on the UI goroutine only.
type login struct {
	runs, want int
	da         *oauth2.DeviceAuthResponse
	state      State
	msg        string
	cancel     context.CancelFunc
}

func (v LoginView) body(ctx *gift.Context) gift.View {
	mem := ctx.State("login", (*login)(nil))
	st := mem.Get()
	if st == nil {
		st = &login{want: 1}
		mem.Set(st)
	}
	changed := ctx.State("changed", 0)
	ctx.Read(changed)
	bump := func() { changed.Set(changed.Get() + 1) }
	ctx.OnUnmount("cancel", func() {
		if st.cancel != nil {
			st.cancel()
		}
	})
	if st.runs < st.want {
		st.runs = st.want
		v.start(ctx.Request(), st, bump)
	}
	t := v.texts.withDefaults()
	msg := st.msg
	if st.state == Failed && msg == "" {
		msg = t.Failed
	}
	return Device(st.da).State(st.state).Message(msg).Texts(v.texts).
		OnRestart(func() { st.want++; bump() })
}

// start begins a run of the flow, ending the one before.
func (v LoginView) start(tok gift.Token, st *login, bump func()) {
	if st.cancel != nil {
		st.cancel()
	}
	st.da, st.state, st.msg = nil, Waiting, ""
	parent := v.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	st.cancel = cancel
	conf, opts, done, failed := v.conf, v.opts, v.done, v.failed
	t := v.texts.withDefaults()
	fail := func(err error) {
		st.state = Failed
		var re *oauth2.RetrieveError
		switch {
		case errors.As(err, &re) && re.ErrorCode == "expired_token",
			errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
			st.state = Expired
		case errors.As(err, &re) && re.ErrorCode == "access_denied":
			st.msg = t.Declined
		default:
			if failed != nil {
				failed(err)
			}
		}
	}
	go func() {
		da, err := conf.DeviceAuth(ctx, opts...)
		if err != nil {
			tok.Post(func() { fail(err); bump() })
			return
		}
		tok.Post(func() { st.da = da; bump() })
		tk, err := conf.DeviceAccessToken(ctx, da, opts...)
		tok.Post(func() {
			if err != nil {
				fail(err)
			} else {
				st.state = Done
				if done != nil {
					done(tk)
				}
			}
			bump()
		})
	}()
}
