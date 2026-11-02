package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// loopback is the provider of [Loopback] callbacks: a server on the loopback
// interface for the redirect, and the system browser for the flow.
type loopback struct{}

func (loopback) Supports(c Callback) bool { return c.kind == LoopbackCallback && canBrowse }

// formLimit bounds the body of a form_post redirect, which carries a code,
// a state and perhaps an ID token.
const formLimit = 64 << 10

func (loopback) Run(ctx context.Context, f Flow, c Callback) (Return, error) {
	ln, err := listenLoopback(ctx)
	if err != nil {
		return Return{}, err
	}
	host := ln.Addr().String()
	got := make(chan *url.URL, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A page the browser was sent to by a name that resolves to
			// the loopback, DNS rebinding, carries that name as its host.
			if r.Host != host {
				http.Error(w, "wrong host", http.StatusMisdirectedRequest)
				return
			}
			if r.URL.Path != c.path {
				http.NotFound(w, r)
				return
			}
			u := &url.URL{Scheme: "http", Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
			switch r.Method {
			case http.MethodGet, http.MethodHead:
			case http.MethodPost:
				// response_mode=form_post, which Sign in with Apple
				// uses: the parameters come as a form, and the return
				// carries them in its query all the same.
				r.Body = http.MaxBytesReader(w, r.Body, formLimit)
				if err := r.ParseForm(); err != nil {
					http.Error(w, "bad form", http.StatusBadRequest)
					return
				}
				q := r.URL.Query()
				for k, vs := range r.PostForm {
					q[k] = append(q[k], vs...)
				}
				u.RawQuery = q.Encode()
			default:
				w.Header().Set("Allow", "GET, HEAD, POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			select {
			case got <- u:
			default:
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if f.Landing != "" {
				http.Redirect(w, r, f.Landing, http.StatusSeeOther)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(landingPage))
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		// Shutdown waits for the answer to the redirect, the page the
		// browser shows, and closes the connections the browser keeps.
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(sctx) != nil {
			_ = srv.Close()
		}
	}()

	target, err := f.Start(ctx, "http://"+host+c.path)
	if err != nil {
		return Return{}, err
	}
	if err := OpenURL(target); err != nil {
		return Return{}, err
	}
	select {
	case u := <-got:
		return Return{URL: u}, nil
	case <-ctx.Done():
		return Return{}, context.Cause(ctx)
	}
}

// listenLoopback listens on a free port of 127.0.0.1, or of ::1 on a machine
// without IPv4, with the socket options of RFC 8252 appendix B: no address
// reuse, so that no other process can bind the same port and take the
// redirect.
func listenLoopback(ctx context.Context) (net.Listener, error) {
	lc := net.ListenConfig{Control: exclusive}
	ln, err := lc.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err == nil {
		return ln, nil
	}
	if ln6, err6 := lc.Listen(ctx, "tcp6", "[::1]:0"); err6 == nil {
		return ln6, nil
	}
	return nil, errors.Join(errors.New("gift/web: no loopback port"), err)
}

// landingPage is what the browser shows once the redirect has arrived.
const landingPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Done</title>
<style>
body{margin:0;min-height:100vh;display:grid;place-items:center;font:17px/1.4 system-ui,sans-serif;
color:#1c1c1e;background:#f2f2f7;text-align:center}
@media (prefers-color-scheme:dark){body{color:#f2f2f7;background:#1c1c1e}}
h1{font-size:28px;margin:0 0 8px}
</style></head>
<body><main><h1>Done</h1><p>You can close this tab and return to the application.</p></main></body></html>
`
