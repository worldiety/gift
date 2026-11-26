package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The platform input method half of the text widgets: what a field tells a
// system keyboard through [gift.TextInputClient], and what it does with the
// [gift.EventTextReplace] the keyboard sends back. Like the rest of the field
// tests these go through the harness — the state is read with
// [gift.App.TextInput] and the edit delivered with
// [gifttest.Harness.ReplaceText], which is exactly the pair of calls a backend
// makes — because what is worth catching is whether the two meet in the node,
// in the right space.
//
// The arithmetic of the replacement itself is tested on the editor directly,
// in TestReplaceAroundClampsAndWidensToWholeRunes.

// textInput is the input method state of the focused node, failing the test
// when there is none.
func textInput(t *testing.T, h *gifttest.Harness) gift.TextInputState {
	t.Helper()
	st, ok := h.App().TextInput()
	if !ok {
		t.Fatal("the focused node reports no text input state")
	}
	return st
}

// --- the state ---------------------------------------------------------------

// TestAFieldReportsTheTextAroundTheCaretAndTheSelection. A selection is in
// neither string, because the next replacement replaces it.
func TestAFieldReportsTheTextAroundTheCaretAndTheSelection(t *testing.T) {
	f := newField(t, "hello world", nil)
	f.node().Focus()

	st := textInput(t, f.h)
	if st.Before != "hello world" || st.After != "" || st.Secure {
		t.Fatalf("with the caret at the end the state is %+v", st)
	}
	f.ed.SetCaret(5)
	if st := textInput(t, f.h); st.Before != "hello" || st.After != " world" {
		t.Fatalf("with the caret after hello the state is %q | %q", st.Before, st.After)
	}
	f.ed.SetSelection(2, 7)
	if st := textInput(t, f.h); st.Before != "he" || st.After != "orld" {
		t.Fatalf("with llo w selected the state is %q | %q, want the selection in neither", st.Before, st.After)
	}
}

// TestNothingFocusedIsNoTextInput: a backend asks before raising a keyboard,
// and an unfocused field, or a focused button, is not something to type into.
func TestNothingFocusedIsNoTextInput(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(
			ui.TextField(ui.NewTextEditor("x")).Font(loadTestFont(t)).Key("field"),
			ui.Button(ui.Text("ok").Font(loadTestFont(t)), nil).Key("button"),
		),
		Size: geom.Sz(400, 200),
		Font: loadTestFont(t),
	})
	if _, ok := h.App().TextInput(); ok {
		t.Fatal("nothing is focused and there is a text input state")
	}
	h.Find(gifttest.ByKey("button")).Focus()
	if _, ok := h.App().TextInput(); ok {
		t.Fatal("a focused button reports a text input state")
	}
}

// TestTheReportedCaretIsThePaintedCaret. The state is in device space when the
// App hands it out, and it has to be the rectangle the user sees: a system
// keyboard that keeps "the caret" clear of itself keeps clear whatever this
// reports. It follows typing, including typing that scrolls the field.
func TestTheReportedCaretIsThePaintedCaret(t *testing.T) {
	f := newField(t, "", nil)
	f.node().Focus()
	check := func(when string) {
		t.Helper()
		op, ok := caretOp(f.h)
		if !ok {
			t.Fatalf("%s: no caret painted", when)
		}
		if got := textInput(t, f.h).Caret; got != op.Bounds {
			t.Fatalf("%s: the state reports the caret at %v, the field painted it at %v", when, got, op.Bounds)
		}
	}
	check("empty")
	before := textInput(t, f.h).Caret

	f.h.TypeText("grün")
	check("after typing")
	if after := textInput(t, f.h).Caret; !(after.Min.X > before.Min.X) {
		t.Fatalf("typing four characters left the caret at %v, it was at %v", after, before)
	}

	// Far wider than the field: the text scrolls, and the caret stays at the
	// right edge of the content box rather than running off to the right.
	f.h.ReplaceText(0, 0, strings.Repeat("wide ", 20))
	check("after a scroll")
	if b := f.node().Bounds(); textInput(t, f.h).Caret.Max.X > b.Max.X {
		t.Fatalf("the caret of a scrolled field is reported at %v, outside the field %v", textInput(t, f.h).Caret, b)
	}
}

// TestASecureFieldReportsSecureWithItsTextAndACaret, revealed or not: a
// password the user chose to look at is still a password, and a keyboard that
// deletes the character before the caret has to have the text.
func TestASecureFieldReportsSecureWithItsTextAndACaret(t *testing.T) {
	for _, revealed := range []bool{false, true} {
		f := newSecureField(t, "secret", func(v ui.TextFieldView) ui.TextFieldView { return v.Revealed(revealed) })
		st := textInput(t, f.h)
		if !st.Secure || st.Before != "secret" || st.After != "" {
			t.Fatalf("revealed=%v: a secure field reports %+v", revealed, st)
		}
		if st.Caret.Height() <= 0 || st.Caret.Width() <= 0 {
			t.Fatalf("revealed=%v: a secure field reports no caret, %v", revealed, st.Caret)
		}
	}
}

// --- the replacement ---------------------------------------------------------

// TestACommitInsertsAtTheCaret is the plain case: a system keyboard committing
// a word, or one character, with nothing to delete.
func TestACommitInsertsAtTheCaret(t *testing.T) {
	f := newField(t, "ab", nil)
	f.node().Focus()
	f.ed.SetCaret(1)
	f.h.ReplaceText(0, 0, "xyz")
	if got := f.ed.Text(); got != "axyzb" {
		t.Fatalf("the document is %q, want %q", got, "axyzb")
	}
	if got := f.ed.Caret(); got != 4 {
		t.Fatalf("the caret is at %d, want 4, after the insertion", got)
	}
	if len(f.change) != 1 || f.change[0] != "axyzb" {
		t.Fatalf("OnChange saw %q, want one call with the new text", f.change)
	}
}

// TestAnAutocorrectionReplacesTheWordBeforeTheCaret: "hte" becomes "the " —
// three bytes before, none after, and the corrected word with its space.
func TestAnAutocorrectionReplacesTheWordBeforeTheCaret(t *testing.T) {
	f := newField(t, "say hte", nil)
	f.node().Focus()
	f.h.ReplaceText(3, 0, "the ")
	if got := f.ed.Text(); got != "say the " {
		t.Fatalf("the document is %q, want %q", got, "say the ")
	}
	if st := textInput(t, f.h); st.Before != "say the " || st.After != "" {
		t.Fatalf("after the correction the state is %q | %q", st.Before, st.After)
	}
}

// TestADeletionWithoutTextIsABackspace, and one with a selection removes the
// selection as well, as every replacement does.
func TestADeletionWithoutTextIsABackspace(t *testing.T) {
	f := newField(t, "abcd", nil)
	f.node().Focus()
	f.h.ReplaceText(1, 0, "")
	if got := f.ed.Text(); got != "abc" {
		t.Fatalf("a deletion of one byte before the caret left %q", got)
	}
	f.ed.SetSelection(1, 2)
	f.h.ReplaceText(0, 0, "")
	if got := f.ed.Text(); got != "ac" {
		t.Fatalf("an empty replacement over a selection left %q, want the selection gone", got)
	}
	// Nothing to delete and nothing to insert is no edit, and no callback.
	n := len(f.change)
	f.ed.SetCaret(0)
	f.h.ReplaceText(1, 0, "")
	if got := f.ed.Text(); got != "ac" || len(f.change) != n {
		t.Fatalf("a backspace at the start changed the document to %q and called OnChange %d time(s)",
			got, len(f.change)-n)
	}
}

// TestAReplacementNeverSplitsAMultiByteCharacter. A count that ends inside a
// multi byte character, as a backend that rounded a platform's count wrong
// could send, takes the whole character, and a count past the document stops
// at its end.
func TestAReplacementNeverSplitsAMultiByteCharacter(t *testing.T) {
	f := newField(t, "aä😀b", nil)
	f.node().Focus()
	f.ed.SetCaret(len("aä😀"))

	f.h.ReplaceText(1, 0, "")
	if got := f.ed.Text(); got != "aäb" {
		t.Fatalf("one byte of a four byte emoji deleted leaves %q (% x), want the emoji gone", got, got)
	}
	f.h.ReplaceText(1, 0, "ö")
	if got := f.ed.Text(); got != "aöb" {
		t.Fatalf("one byte of an umlaut replaced leaves %q (% x)", got, got)
	}
	f.h.ReplaceText(100, 100, "x")
	if got := f.ed.Text(); got != "x" {
		t.Fatalf("counts past both ends leave %q, want only the insertion", got)
	}
	if got := f.ed.Caret(); got != 1 {
		t.Fatalf("the caret is at %d, want 1", got)
	}
}

// TestABoundFieldPublishesAReplacement: the binding is written by the edit the
// way it is by a keystroke.
func TestABoundFieldPublishesAReplacement(t *testing.T) {
	var value *gift.State[string]
	ed := ui.NewTextEditor("")
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			value = ctx.State("value", "")
			return ui.TextField(ed).Font(loadTestFont(t)).FontSize(16).
				Bind(value.Binding()).Key("field")
		},
		Size: geom.Sz(400, 200),
		Font: loadTestFont(t),
	})
	h.Find(gifttest.ByKey("field")).Focus()
	h.ReplaceText(0, 0, "Grüße")
	if got := value.Get(); got != "Grüße" {
		t.Fatalf("the bound state is %q, want %q", got, "Grüße")
	}
}

// TestAFieldKeepsOnlyTheFirstLineOfAReplacement, exactly as of a paste, and
// still applies the deletion; an area keeps the line break.
func TestAFieldKeepsOnlyTheFirstLineOfAReplacement(t *testing.T) {
	var submitted int
	f := newField(t, "teh", func(v ui.TextFieldView) ui.TextFieldView {
		return v.OnSubmit(func(string) { submitted++ })
	})
	f.node().Focus()
	f.h.ReplaceText(3, 0, "the\nnext")
	if got := f.ed.Text(); got != "the" {
		t.Fatalf("a replacement with a line break left %q in a single line field, want %q", got, "the")
	}
	f.h.ReplaceText(0, 0, "\n")
	if got := f.ed.Text(); got != "the" || submitted != 0 {
		t.Fatalf("a lone line break changed the field to %q and submitted %d time(s); "+
			"the return key arrives as a key", got, submitted)
	}

	a := newArea(t, "one", nil)
	a.h.ReplaceText(0, 0, "\ntwo")
	if got := a.ed.Text(); got != "one\ntwo" {
		t.Fatalf("the area holds %q, want the line break kept", got)
	}
}

// TestADisabledFieldTakesNoReplacement, as it takes no rune.
func TestADisabledFieldTakesNoReplacement(t *testing.T) {
	disabled := false
	ed := ui.NewTextEditor("abc")
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			return ui.TextField(ed).Font(loadTestFont(t)).Disabled(disabled).Key("field")
		},
		Size: geom.Sz(400, 200),
		Font: loadTestFont(t),
	})
	h.Find(gifttest.ByKey("field")).Focus()
	disabled = true
	h.App().Invalidate()
	h.Settle()
	h.ReplaceText(1, 0, "x")
	if got := ed.Text(); got != "abc" {
		t.Fatalf("a disabled field took a replacement: %q", got)
	}
}

// TestASecureFieldShowsTheLastCharacterOfAReplacement, as it does of a typed
// one, and hides it again at the next edit that inserts nothing.
func TestASecureFieldShowsTheLastCharacterOfAReplacement(t *testing.T) {
	f := newSecureField(t, "", func(v ui.TextFieldView) ui.TextFieldView { return v.ShowsLastCharacter(true) })
	f.h.ReplaceText(0, 0, "abc")
	if got := f.ed.Text(); got != "abc" {
		t.Fatalf("the document is %q", got)
	}
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 2 || len(ids) != 1 {
		t.Fatalf("right after the replacement the field draws %d dots and %v, want two dots and the c",
			fieldDots(f.h), ids)
	}
	f.h.ReplaceText(1, 0, "")
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 2 || len(ids) != 0 {
		t.Fatalf("after a deletion the field draws %d dots and %v, want two dots", fieldDots(f.h), ids)
	}
	f.h.Advance(ui.SecureRevealDuration + 100*time.Millisecond)
}

// --- the area ----------------------------------------------------------------

// TestAnAreaReportsItsTextAndThePaintedCaret, on the second line, which is
// what tells a caret computed from the area's own line layout from one copied
// from the single line field.
func TestAnAreaReportsItsTextAndThePaintedCaret(t *testing.T) {
	f := newArea(t, "one", nil)
	first := textInput(t, f.h).Caret
	f.h.ReplaceText(0, 0, "\ntwo")
	st := textInput(t, f.h)
	if st.Before != "one\ntwo" || st.After != "" || st.Secure {
		t.Fatalf("the area reports %+v", st)
	}
	if got := f.caret(t); st.Caret != got {
		t.Fatalf("the area reports its caret at %v and painted it at %v", st.Caret, got)
	}
	if !(st.Caret.Min.Y > first.Min.Y) {
		t.Fatalf("the caret on the second line is reported at %v, the first line's at %v", st.Caret, first)
	}
	f.h.ReplaceText(3, 0, "2")
	if got := f.ed.Text(); got != "one\n2" {
		t.Fatalf("a replacement in the area left %q", got)
	}
	if len(f.change) == 0 || f.change[len(f.change)-1] != "one\n2" {
		t.Fatalf("OnChange saw %q", f.change)
	}
}

// TestAHugeAreaReportsAWindowAroundTheCaret. The state is handed to a platform
// that copies it on every caret move, so a long document is seen through a
// window; it is cut at a rune boundary and still meets the caret exactly.
func TestAHugeAreaReportsAWindowAroundTheCaret(t *testing.T) {
	doc := strings.Repeat("ä", 10000) // 20000 bytes
	f := newArea(t, doc, nil)
	f.ed.SetCaret(len(doc) / 2)
	st := textInput(t, f.h)
	if len(st.Before) == 0 || len(st.Before) > 4096 || len(st.After) == 0 || len(st.After) > 4096 {
		t.Fatalf("a 20000 byte document reports %d bytes before and %d after", len(st.Before), len(st.After))
	}
	if !strings.HasSuffix(doc[:len(doc)/2], st.Before) || !strings.HasPrefix(doc[len(doc)/2:], st.After) {
		t.Fatal("the window does not meet the caret")
	}
	if strings.Trim(st.Before+st.After, "ä") != "" {
		t.Fatal("the window splits an umlaut")
	}
}

// --- the editor --------------------------------------------------------------

// TestReplaceAroundClampsAndWidensToWholeRunes is the arithmetic of
// [ui.TextEditor.ReplaceAround] without a field around it.
func TestReplaceAroundClampsAndWidensToWholeRunes(t *testing.T) {
	cases := []struct {
		doc           string
		lo, hi        int
		before, after int
		s, want       string
		caret         int
	}{
		{"abc", 3, 3, 0, 0, "d", "abcd", 4},
		{"abc", 1, 1, 1, 1, "X", "Xc", 1},
		{"abc", 1, 2, 0, 0, "X", "aXc", 2},
		{"abc", 1, 2, 1, 1, "", "", 0},
		{"abc", 1, 1, -5, -5, "", "abc", 1},
		{"aäb", 3, 3, 1, 0, "", "ab", 1},
		{"aäb", 1, 1, 0, 1, "", "ab", 1},
		{"😀😀", 4, 4, 3, 3, "", "", 0},
		{"abc", 1, 1, 99, 99, "z", "z", 1},
	}
	for _, c := range cases {
		e := ui.NewTextEditor(c.doc)
		e.SetSelection(c.lo, c.hi)
		changed := e.ReplaceAround(c.before, c.after, c.s)
		if got := e.Text(); got != c.want || e.Caret() != c.caret {
			t.Errorf("%q [%d,%d] -%d/+%d %q: got %q caret %d, want %q caret %d",
				c.doc, c.lo, c.hi, c.before, c.after, c.s, got, e.Caret(), c.want, c.caret)
		}
		if changed != (c.want != c.doc) {
			t.Errorf("%q -%d/+%d %q reported changed=%v", c.doc, c.before, c.after, c.s, changed)
		}
	}
}

// --- the drawn keyboard ------------------------------------------------------

// TestTheDrawnKeyboardStepsAsideForAPlatformOne. While a backend reports a
// system keyboard, gift's own stays down — and gives the form its room back,
// because the platform makes room for its own — and the field is typed into
// through replacements; when the system keyboard goes, the drawn one serves
// the request that is still standing.
func TestTheDrawnKeyboardStepsAsideForAPlatformOne(t *testing.T) {
	f := newKeyboard(t, true)
	f.field().Focus()
	if !f.keyboardIsShowing() {
		t.Fatal("no drawn keyboard to begin with")
	}
	form := f.h.Find(gifttest.ByKey("form"))

	f.h.App().SetPlatformKeyboardActive(true)
	f.h.Settle()
	if f.keyboardIsShowing() {
		t.Fatal("a platform keyboard is up and the drawn one is still on the screen")
	}
	if got := form.LayoutBounds().Height(); got != kbWindowH {
		t.Fatalf("the form is %v tall under a platform keyboard, want its room back, %v", got, kbWindowH)
	}
	f.h.AssertFocus(gifttest.ByKey("field"))
	f.h.ReplaceText(0, 0, "hallo")
	if got := f.ed.Text(); got != "hallo" {
		t.Fatalf("the field holds %q", got)
	}

	f.h.App().SetPlatformKeyboardActive(false)
	f.h.Settle()
	if !f.keyboardIsShowing() {
		t.Fatal("the platform keyboard went and the drawn one did not come back for the focused field")
	}
	f.h.Advance(time.Second)
}

// TestAPlatformKeyboardIsAskedForWithoutTheKioskSwitch. A backend that
// reports a system keyboard with [gift.App.SetPlatformKeyboardAvailable] is
// the statement about the machine the switch would make: a field that gains
// the focus asks for a keyboard, and the drawn one serves it only where the
// backend does not report that the system keyboard took it. The Ebitengine
// backend reports that in the same input phase as the focus, before the
// build, so the drawn keyboard never shows for a field the system keyboard
// serves; see TestIMEDrawnKeyboardDoesNotFlashForAFieldTheSystemKeyboardTakes
// in that package. A focus moved later, inside [gift.App.Update] by a timer
// or a build, still shows it for one frame. This test sets the report a tick
// late on purpose, to see the drawn keyboard step aside. Without the report
// and without the switch, nothing asks, which is every desktop.
func TestAPlatformKeyboardIsAskedForWithoutTheKioskSwitch(t *testing.T) {
	f := newKeyboard(t, false)
	f.field().Focus()
	if f.h.App().SoftKeyboardRequested() {
		t.Fatal("with neither the switch nor a platform keyboard a field asked for a keyboard")
	}
	f.h.Find(gifttest.ByKey("done")).Focus()

	f.h.App().SetPlatformKeyboardAvailable(true)
	f.h.Settle()
	f.field().Focus()
	if !f.h.App().SoftKeyboardRequested() {
		t.Fatal("a platform keyboard is available and the focused field did not ask for one")
	}
	f.h.App().SetPlatformKeyboardActive(true)
	f.h.Settle()
	if f.keyboardIsShowing() {
		t.Fatal("the system keyboard serves the field and the drawn one is on the screen")
	}
	f.h.App().SetPlatformKeyboardActive(false)
	f.h.Settle()
	if !f.keyboardIsShowing() {
		t.Fatal("a field the system keyboard declined got no drawn keyboard")
	}
	f.h.Find(gifttest.ByKey("done")).Focus()
	if f.h.App().SoftKeyboardRequested() {
		t.Fatal("the request outlived the focus")
	}
	f.h.Advance(time.Second)
}

// TestAWithdrawnPlatformKeyboardLeavesNoRequestBehind: a backend that takes
// its keyboard back while a field holds the focus must not leave that field's
// request standing, or the drawn keyboard would come up for a button once the
// backend offers its keyboard again. The field takes the request back on blur
// whatever is offered then.
func TestAWithdrawnPlatformKeyboardLeavesNoRequestBehind(t *testing.T) {
	f := newKeyboard(t, false)
	f.h.App().SetPlatformKeyboardAvailable(true)
	f.h.Settle()
	f.field().Focus()
	if !f.h.App().SoftKeyboardRequested() {
		t.Fatal("a platform keyboard is available and the focused field did not ask for one")
	}
	f.h.App().SetPlatformKeyboardAvailable(false)
	f.h.Settle()
	f.h.Find(gifttest.ByKey("done")).Focus()
	if f.h.App().SoftKeyboardRequested() {
		t.Fatal("the request outlived the focus after the platform keyboard was withdrawn")
	}
	f.h.App().SetPlatformKeyboardAvailable(true)
	f.h.Settle()
	if f.keyboardIsShowing() {
		t.Fatal("the drawn keyboard came up for a button")
	}
	f.h.Advance(time.Second)
}

// TestASecureFieldShowsTheLastCharacterWhereThePlatformHasAKeyboard, the
// touch convention, as it does with the kiosk switch on.
func TestASecureFieldShowsTheLastCharacterWhereThePlatformHasAKeyboard(t *testing.T) {
	f := newSecureField(t, "", nil)
	f.h.App().SetPlatformKeyboardAvailable(true)
	f.h.Settle()
	f.node().Focus()
	f.h.ReplaceText(0, 0, "ab")
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 1 || len(ids) != 1 {
		t.Fatalf("right after the replacement the field draws %d dots and %v, want one dot and the b",
			fieldDots(f.h), ids)
	}
	f.h.Advance(ui.SecureRevealDuration + 100*time.Millisecond)
}

// TestAPlatformKeyboardReportedLateReachesAMemoisedField: the build reads
// [gift.BuildContext.PlatformKeyboardAvailable] for the field's default, so a
// backend that reports it after the field was mounted must reach it through a
// [gift.Memo] as well.
func TestAPlatformKeyboardReportedLateReachesAMemoisedField(t *testing.T) {
	ed := ui.NewTextEditor("")
	font := loadTestFont(t)
	h := gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.Box(), gift.Memo("m", 0, func(*gift.Context, int) gift.View {
			return ui.VStack(ui.SecureField(ed).Font(font).FontSize(16).Frame(200, geom.Unbounded()).Key("field")).Padding(20)
		})),
		Size: geom.Sz(400, 200),
		Font: font,
	})
	h.App().SetPlatformKeyboardAvailable(true)
	h.Settle()
	h.Find(gifttest.ByKey("field")).Focus()
	h.ReplaceText(0, 0, "ab")
	if ids := fieldGlyphs(h); fieldDots(h) != 1 || len(ids) != 1 {
		t.Fatalf("the memoised field draws %d dots and %v, want one dot and the b", fieldDots(h), ids)
	}
	h.Advance(ui.SecureRevealDuration + 100*time.Millisecond)
}
