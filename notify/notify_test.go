package notify

import (
	"bytes"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"testing"
)

// The portable half: the single attempt at opening, the validation, the cap,
// and the way a click reaches Post. The platform below is a fake.

type fakePlatform struct {
	shown     []Notification
	withdrawn []string
}

func (p *fakePlatform) show(n Notification) error { p.shown = append(p.shown, n); return nil }
func (p *fakePlatform) withdraw(id string) error  { p.withdrawn = append(p.withdrawn, id); return nil }
func (p *fakePlatform) authorize(done func(bool)) { done(true) }

func TestAFailedOpenIsTriedOnceAndEveryShowSaysUnsupported(t *testing.T) {
	var logs bytes.Buffer
	opens := 0
	n := newNotifier(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))}, func(openConfig) (platform, error) {
		opens++
		return nil, ErrUnsupported
	})
	for range 3 {
		if err := n.Show(Notification{Title: "t"}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("error %v, want ErrUnsupported", err)
		}
	}
	if err := n.Withdraw("a"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Withdraw: %v", err)
	}
	if opens != 1 {
		t.Fatalf("opened %d times", opens)
	}
	if c := strings.Count(logs.String(), "notifications unavailable"); c != 1 {
		t.Fatalf("logged the failure %d times:\n%s", c, logs.String())
	}
}

func TestShowRefusesWhatNoSystemCanDisplay(t *testing.T) {
	p := &fakePlatform{}
	n := newNotifier(Config{}, func(openConfig) (platform, error) { return p, nil })
	for _, bad := range []Notification{
		{},
		{Title: "a\xffb"},
		{Title: "t", Body: "a\x00b"},
		{ID: "\xff", Title: "t"},
	} {
		if err := n.Show(bad); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	if len(p.shown) != 0 {
		t.Fatalf("%d refused notifications reached the platform", len(p.shown))
	}
}

func TestLongTextIsCutAtARuneBoundary(t *testing.T) {
	p := &fakePlatform{}
	n := newNotifier(Config{}, func(openConfig) (platform, error) { return p, nil })
	body := strings.Repeat("a", MaxBytes-1) + "ä" + "tail"
	if err := n.Show(Notification{Title: "t", Body: body}); err != nil {
		t.Fatal(err)
	}
	if got := p.shown[0].Body; got != strings.Repeat("a", MaxBytes-1) {
		t.Fatalf("kept %d bytes ending in %q", len(got), got[len(got)-3:])
	}
}

func TestAClickIsPostedAndANilPostDropsIt(t *testing.T) {
	var activated func(func())
	open := func(c openConfig) (platform, error) { activated = c.activated; return &fakePlatform{}, nil }

	var posted []func()
	n := newNotifier(Config{Post: func(fn func()) { posted = append(posted, fn) }}, open)
	n.Show(Notification{Title: "t"})
	ran := false
	activated(func() { ran = true })
	if len(posted) != 1 || ran {
		t.Fatalf("posted %d, ran %v; want it posted and not run", len(posted), ran)
	}

	var logs bytes.Buffer
	n = newNotifier(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))}, open)
	n.Show(Notification{Title: "t"})
	activated(func() { t.Fatal("ran without Post") })
	activated(func() { t.Fatal("ran without Post") })
	if c := strings.Count(logs.String(), "Config.Post is nil"); c != 1 {
		t.Fatalf("logged the dropped click %d times", c)
	}
}

func TestOtherPlatformsAreUnsupported(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Skip("this platform has notifications; see its own test file")
	}
	if err := New(Config{}).Show(Notification{Title: "t"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error %v, want ErrUnsupported", err)
	}
}

func TestAuthorizeAnswersThroughPost(t *testing.T) {
	var posted []func()
	post := func(fn func()) { posted = append(posted, fn) }

	var answers []bool
	n := newNotifier(Config{Post: post}, func(openConfig) (platform, error) { return &fakePlatform{}, nil })
	n.Authorize(func(granted bool) { answers = append(answers, granted) })
	n = newNotifier(Config{Post: post}, func(openConfig) (platform, error) { return nil, ErrUnsupported })
	n.Authorize(func(granted bool) { answers = append(answers, granted) })
	if len(answers) != 0 {
		t.Fatal("an answer ran before it was posted")
	}
	for _, fn := range posted {
		fn()
	}
	if len(answers) != 2 || !answers[0] || answers[1] {
		t.Fatalf("answers %v, want [true false]", answers)
	}
	newNotifier(Config{}, func(openConfig) (platform, error) { return &fakePlatform{}, nil }).Authorize(nil)
}
