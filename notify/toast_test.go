package notify

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToastXMLIsTheGenericTemplate(t *testing.T) {
	x, cut := toastXML("t1", "Import finished", "1 024 pictures")
	want := `<toast launch="t1"><visual><binding template="ToastGeneric">` +
		`<text>Import finished</text><text>1 024 pictures</text></binding></visual></toast>`
	if x != want || cut {
		t.Fatalf("got %s (cut %v)\nwant %s", x, cut, want)
	}
	if x, _ := toastXML("t1", "Title", ""); strings.Count(x, "<text>") != 1 {
		t.Fatalf("an empty body still got a text element: %s", x)
	}
}

func TestToastXMLEscapesAndRemovesWhatXMLForbids(t *testing.T) {
	x, _ := toastXML(`a"b`, "<b>bold</b> & 'co'", "bell\x07here\ttab")
	for _, want := range []string{
		`launch="a&quot;b"`,
		"<text>&lt;b&gt;bold&lt;/b&gt; &amp; &apos;co&apos;</text>",
		"<text>bell here\ttab</text>",
	} {
		if !strings.Contains(x, want) {
			t.Errorf("%s lacks %s", x, want)
		}
	}
}

// TestToastXMLFitsEvenWhenEscapingQuintuplesTheText is the case MaxBytes
// alone does not cover: 4 KiB of ampersands are 20 KiB of XML.
func TestToastXMLFitsEvenWhenEscapingQuintuplesTheText(t *testing.T) {
	for _, tc := range []struct{ title, body string }{
		{"t", strings.Repeat("&", MaxBytes)},
		{strings.Repeat("ä", MaxBytes/2), strings.Repeat("b", MaxBytes)},
		{strings.Repeat("<", MaxBytes), ""},
	} {
		x, cut := toastXML("launch", tc.title, tc.body)
		if !cut || len(x) > maxToastBytes {
			t.Errorf("%d bytes, cut %v", len(x), cut)
		}
		if !utf8.ValidString(x) || strings.Contains(x, "&am<") || strings.HasSuffix(strings.Split(x, "</text>")[0], "&") {
			t.Errorf("cut inside a rune or an entity: ...%s", x[len(x)-60:])
		}
	}
}

func TestCheckAUMID(t *testing.T) {
	if err := checkAUMID("com.example.Photos"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "com\\example", "com example", strings.Repeat("a", maxAUMID+1), "a\xff"} {
		if err := checkAUMID(bad); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestActivatorCLSIDIsStableAndAVersion5UUID(t *testing.T) {
	a, b := activatorCLSID("com.example.Photos"), activatorCLSID("com.example.Photos")
	if a != b {
		t.Fatal("the same AppID gave two class ids")
	}
	if a == activatorCLSID("com.example.Mail") {
		t.Fatal("two AppIDs share a class id")
	}
	s := guidString(a)
	if len(s) != 38 || s[0] != '{' || s[37] != '}' || s[15] != '5' || !strings.ContainsAny(s[20:21], "89AB") {
		t.Fatalf("%s is not a braced version 5 UUID", s)
	}
}
