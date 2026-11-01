package notify

import (
	"errors"
	"reflect"
	"testing"
)

// The notification protocol is tested against a server that lives in the
// test: fakeServer records the calls, hands out ids, and the tests deliver
// the signals by calling freedesktop.signal themselves, as the reader
// goroutine would.

type call struct {
	member string
	args   []any
}

type fakeServer struct {
	caps    []any
	capsErr error
	nextID  uint32
	calls   []call
}

func (s *fakeServer) call(dest string, path objectPath, iface, member string, sig signature, args ...any) ([]any, error) {
	s.calls = append(s.calls, call{member, args})
	switch member {
	case "AddMatch", "CloseNotification":
		return nil, nil
	case "GetCapabilities":
		if s.capsErr != nil {
			return nil, s.capsErr
		}
		return []any{s.caps}, nil
	case "Notify":
		if r := args[1].(uint32); r != 0 {
			return []any{r}, nil
		}
		s.nextID++
		return []any{s.nextID}, nil
	}
	return nil, errors.New("unexpected call " + member)
}

func (s *fakeServer) last(member string) []any {
	for i := len(s.calls) - 1; i >= 0; i-- {
		if s.calls[i].member == member {
			return s.calls[i].args
		}
	}
	return nil
}

func newTestFreedesktop(t *testing.T, caps ...any) (*freedesktop, *fakeServer, *[]func()) {
	t.Helper()
	s := &fakeServer{caps: caps}
	var posted []func()
	f, err := newFreedesktop(s, openConfig{appID: "com.example.Photos", appName: "Photos",
		activated: func(fn func()) { posted = append(posted, fn) }})
	if err != nil {
		t.Fatal(err)
	}
	return f, s, &posted
}

func signalMsg(member, sig string, body ...any) *message {
	return &message{typ: msgSignal, path: notifyPath, iface: notifyInterface, member: member, sig: signature(sig), body: body}
}

func TestOpeningSubscribesBeforeItAsks(t *testing.T) {
	_, s, _ := newTestFreedesktop(t, "actions", "body")
	if len(s.calls) != 2 || s.calls[0].member != "AddMatch" || s.calls[1].member != "GetCapabilities" {
		t.Fatalf("calls %v", s.calls)
	}
}

func TestNoServerIsUnsupported(t *testing.T) {
	s := &fakeServer{capsErr: &busError{name: "org.freedesktop.DBus.Error.ServiceUnknown"}}
	_, err := newFreedesktop(s, openConfig{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error %v, want ErrUnsupported", err)
	}
}

func TestNotifySendsWhatTheSpecificationAsksFor(t *testing.T) {
	f, s, _ := newTestFreedesktop(t, "actions", "body")
	if err := f.show(Notification{Title: "Import finished", Body: "1 024 pictures", OnClick: func() {}}); err != nil {
		t.Fatal(err)
	}
	want := []any{"Photos", uint32(0), "", "Import finished", "1 024 pictures",
		[]string{"default", ""},
		[]dictEntry{{key: "desktop-entry", value: variant{sig: "s", value: "com.example.Photos"}}},
		int32(-1)}
	if got := s.last("Notify"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Notify\n got %#v\nwant %#v", got, want)
	}
}

// TestTheDefaultActionNeedsAClickHandlerAndAServerThatTakesActions: without
// either there is no "default" action, because asking for one would make
// some servers show an empty button for a click that does nothing.
func TestTheDefaultActionNeedsAClickHandlerAndAServerThatTakesActions(t *testing.T) {
	f, s, _ := newTestFreedesktop(t, "actions")
	f.show(Notification{Title: "t"})
	if a := s.last("Notify")[5].([]string); len(a) != 0 {
		t.Errorf("no OnClick, actions %v", a)
	}
	f, s, _ = newTestFreedesktop(t, "body")
	f.show(Notification{Title: "t", OnClick: func() {}})
	if a := s.last("Notify")[5].([]string); len(a) != 0 {
		t.Errorf("no actions capability, actions %v", a)
	}
}

func TestTheBodyIsEscapedWhenTheServerParsesMarkup(t *testing.T) {
	f, s, _ := newTestFreedesktop(t, "body", "body-markup")
	f.show(Notification{Title: "a < b", Body: "a < b & c > d"})
	args := s.last("Notify")
	if args[3] != "a < b" {
		t.Errorf("the title was changed to %q; it is never markup", args[3])
	}
	if args[4] != "a &lt; b &amp; c &gt; d" {
		t.Errorf("body %q", args[4])
	}
	f, s, _ = newTestFreedesktop(t, "body")
	f.show(Notification{Title: "t", Body: "a < b"})
	if got := s.last("Notify")[4]; got != "a < b" {
		t.Errorf("a server without body-markup got %q", got)
	}
}

func TestTheSameIDReplaces(t *testing.T) {
	f, s, _ := newTestFreedesktop(t)
	f.show(Notification{ID: "import", Title: "1 %"})
	f.show(Notification{ID: "other", Title: "x"})
	f.show(Notification{ID: "import", Title: "50 %"})
	if r := s.last("Notify")[1]; r != uint32(1) {
		t.Fatalf("replaces_id %v, want 1", r)
	}
	f.show(Notification{Title: "no id"})
	if r := s.last("Notify")[1]; r != uint32(0) {
		t.Fatalf("a notification without ID replaced %v", r)
	}
}

func TestAClickOnTheBodyIsPosted(t *testing.T) {
	f, _, posted := newTestFreedesktop(t, "actions")
	clicked := 0
	f.show(Notification{ID: "a", Title: "t", OnClick: func() { clicked++ }})

	f.signal(signalMsg("ActionInvoked", "us", uint32(1), "other"))
	f.signal(signalMsg("ActionInvoked", "us", uint32(99), "default")) // someone else's
	f.signal(signalMsg("ActionInvoked", "s", "default"))              // malformed
	if len(*posted) != 0 {
		t.Fatalf("%d clicks posted for signals that were not ours", len(*posted))
	}
	f.signal(signalMsg("ActionInvoked", "us", uint32(1), "default"))
	if len(*posted) != 1 {
		t.Fatalf("%d clicks posted, want 1", len(*posted))
	}
	if clicked != 0 {
		t.Fatal("OnClick ran on the reader goroutine instead of being posted")
	}
	(*posted)[0]()
	if clicked != 1 {
		t.Fatal("the posted function is not OnClick")
	}
}

// TestAClosedNotificationIsForgotten: after the server reports it closed, a
// Show with the same ID starts a new notification, and a late click on the
// old id does nothing.
func TestAClosedNotificationIsForgotten(t *testing.T) {
	f, s, posted := newTestFreedesktop(t, "actions")
	f.show(Notification{ID: "a", Title: "t", OnClick: func() {}})
	f.signal(signalMsg("NotificationClosed", "uu", uint32(1), uint32(2)))
	f.signal(signalMsg("ActionInvoked", "us", uint32(1), "default"))
	if len(*posted) != 0 {
		t.Fatal("a click on a closed notification was posted")
	}
	f.show(Notification{ID: "a", Title: "t"})
	if r := s.last("Notify")[1]; r != uint32(0) {
		t.Fatalf("replaces_id %v for a closed notification", r)
	}
}

func TestWithdraw(t *testing.T) {
	f, s, _ := newTestFreedesktop(t)
	f.show(Notification{ID: "a", Title: "t"})
	if err := f.withdraw("a"); err != nil {
		t.Fatal(err)
	}
	if got := s.last("CloseNotification"); !reflect.DeepEqual(got, []any{uint32(1)}) {
		t.Fatalf("CloseNotification %v", got)
	}
	n := len(s.calls)
	if err := f.withdraw("a"); err != nil || len(s.calls) != n {
		t.Fatalf("withdrawing twice: %v, %d calls", err, len(s.calls)-n)
	}
}
