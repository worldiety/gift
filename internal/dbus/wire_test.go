package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The wire format is tested against byte layouts worked out by hand from the
// specification, not only by round trips: a round trip through an encoder and
// a decoder that share a misunderstanding passes, and a notification server
// then disconnects.

// helloReply is a METHOD_RETURN with serial 2, REPLY_SERIAL 1 and the body
// (s) ":1.42", laid out by hand:
//
//	0  'l' 2 0 1            little endian, METHOD_RETURN, no flags, version 1
//	4  0a 00 00 00          body length 10
//	8  02 00 00 00          serial 2
//	12 0f 00 00 00          header field array, 15 bytes from offset 16
//	16 05 01 'u' 00         field 5 REPLY_SERIAL, variant signature "u"
//	20 01 00 00 00          1
//	24 08 01 'g' 00         field 8 SIGNATURE, variant signature "g", aligned to 8
//	28 01 's' 00            "s"
//	31 00                   padding of the header to 8
//	32 05 00 00 00 ":1.42" 00
var helloReply = []byte{
	'l', 2, 0, 1,
	0x0a, 0, 0, 0,
	2, 0, 0, 0,
	0x0f, 0, 0, 0,
	5, 1, 'u', 0,
	1, 0, 0, 0,
	8, 1, 'g', 0,
	1, 's', 0,
	0,
	5, 0, 0, 0, ':', '1', '.', '4', '2', 0,
}

func TestMarshalMatchesTheLayoutOfTheSpecification(t *testing.T) {
	m := &Message{Type: TypeMethodReturn, Serial: 2, ReplySerial: 1, Sig: "s", Body: []any{":1.42"}}
	got, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, helloReply) {
		t.Fatalf("marshal\n got % x\nwant % x", got, helloReply)
	}
}

func TestReadMessageDecodesTheLayoutOfTheSpecification(t *testing.T) {
	m, err := ReadMessage(bytes.NewReader(helloReply))
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != TypeMethodReturn || m.Serial != 2 || m.ReplySerial != 1 || m.Sig != "s" {
		t.Fatalf("header %+v", m)
	}
	if len(m.Body) != 1 || m.Body[0] != ":1.42" {
		t.Fatalf("body %#v", m.Body)
	}
}

// TestReadMessageDecodesBigEndian rewrites helloReply into big endian. A peer
// may send either order; every integer flips and nothing else does.
func TestReadMessageDecodesBigEndian(t *testing.T) {
	b := bytes.Clone(helloReply)
	b[0] = 'B'
	for _, off := range []int{4, 8, 12, 20, 32} {
		binary.BigEndian.PutUint32(b[off:], binary.LittleEndian.Uint32(helloReply[off:]))
	}
	m, err := ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.Serial != 2 || m.ReplySerial != 1 || len(m.Body) != 1 || m.Body[0] != ":1.42" {
		t.Fatalf("big endian decoded as %+v", m)
	}
}

// TestAnEmptyArrayStillPadsToItsElements is the rule that is easiest to get
// wrong: the padding before the first element is written even when there is
// no first element, and it is not counted in the length.
func TestAnEmptyArrayStillPadsToItsElements(t *testing.T) {
	var e encoder
	if err := e.value("a(yv)", []any{}); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0, 0, 0, 0, 0, 0, 0, 0}; !bytes.Equal(e.buf, want) {
		t.Fatalf("got % x, want % x", e.buf, want)
	}
	e = encoder{}
	if err := e.value("as", []string{}); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0, 0, 0, 0}; !bytes.Equal(e.buf, want) {
		t.Fatalf("got % x, want % x", e.buf, want)
	}
}

// TestTheNotifyCallRoundTrips sends the one call gift/notify exists for, with
// every argument type it uses, through the encoder and back.
func TestTheNotifyCallRoundTrips(t *testing.T) {
	m := &Message{
		Type: TypeMethodCall, Serial: 7, Path: notifyPath, Interface: notifyInterface, Member: "Notify",
		Destination: notifyName, Sig: "susssasa{sv}i",
		Body: []any{"Photos", uint32(3), "", "Import finished", "1 024 pictures",
			[]string{"default", ""},
			[]DictEntry{
				{Key: "desktop-entry", Value: Variant{Sig: "s", Value: "com.example.Photos"}},
				{Key: "urgency", Value: Variant{Sig: "y", Value: byte(1)}},
				{Key: "resident", Value: Variant{Sig: "b", Value: true}},
			},
			int32(-1)},
	}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"Photos", uint32(3), "", "Import finished", "1 024 pictures",
		[]any{"default", ""},
		[]DictEntry{
			{Key: "desktop-entry", Value: Variant{Sig: "s", Value: "com.example.Photos"}},
			{Key: "urgency", Value: Variant{Sig: "y", Value: byte(1)}},
			{Key: "resident", Value: Variant{Sig: "b", Value: true}},
		},
		int32(-1)}
	if !reflect.DeepEqual(got.Body, want) {
		t.Fatalf("body\n got %#v\nwant %#v", got.Body, want)
	}
	if got.Path != notifyPath || got.Interface != notifyInterface || got.Member != "Notify" ||
		got.Destination != notifyName || got.Serial != 7 || got.Sig != m.Sig {
		t.Fatalf("header %+v", got)
	}
}

func TestEveryBasicTypeRoundTrips(t *testing.T) {
	sig := Signature("ybnqiuxtdsogv(us)")
	in := []any{byte(0xfe), true, int16(-2), uint16(0xfffe), int32(-3), uint32(0xfffffffd),
		int64(-4), uint64(0xfffffffffffffffc), 2.5, "s", ObjectPath("/a/b_1"), Signature("a{sv}"),
		Variant{Sig: "as", Value: []string{"x"}}, []any{uint32(9), "nine"}}
	b, err := (&Message{Type: TypeSignal, Serial: 1, Path: "/", Interface: "a.b", Member: "C", Sig: sig, Body: in}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	in[12] = Variant{Sig: "as", Value: []any{"x"}} // arrays decode as []any
	if !reflect.DeepEqual(m.Body, in) {
		t.Fatalf("body\n got %#v\nwant %#v", m.Body, in)
	}
}

// TestAByteArrayIsBytesBothWays: "ay" carries pictures — the icon of a
// StatusNotifierItem, the PNG of a menu entry — and is written from and read
// into a []byte rather than an element per byte.
func TestAByteArrayIsBytesBothWays(t *testing.T) {
	var e encoder
	if err := e.value("ay", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if want := []byte{3, 0, 0, 0, 1, 2, 3}; !bytes.Equal(e.buf, want) {
		t.Fatalf("got % x, want % x", e.buf, want)
	}
	b, err := (&Message{Type: TypeSignal, Serial: 1, Path: "/", Interface: "a.b", Member: "C", Sig: "a(iiay)",
		Body: []any{[]any{[]any{int32(1), int32(1), []byte{0xff, 1, 2, 3}}}}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ReadMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Body[0].([]any)[0].([]any)[2]; !reflect.DeepEqual(got, []byte{0xff, 1, 2, 3}) {
		t.Fatalf("decoded as %#v", got)
	}
	if err := (&encoder{}).value("as", []byte{1}); err == nil {
		t.Fatal("a []byte was written as an array of strings")
	}
}

func TestTheEncoderRefusesWhatABusWouldDisconnectFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  string
		v    []any
		want string
	}{
		{"invalid UTF-8", "s", []any{"a\xffb"}, "UTF-8"},
		{"NUL in a string", "s", []any{"a\x00b"}, "NUL"},
		{"object path with trailing slash", "o", []any{ObjectPath("/a/")}, "object path"},
		{"object path with a dash", "o", []any{ObjectPath("/a-b")}, "object path"},
		{"wrong Go type", "u", []any{int32(1)}, "cannot be written"},
		{"variant of two types", "v", []any{Variant{Sig: "ss", Value: "x"}}, "single complete type"},
		{"too few values", "ss", []any{"x"}, "more types"},
		{"too many values", "s", []any{"x", "y"}, "fewer types"},
		{"unclosed struct", "(s", []any{[]any{"x"}}, "unclosed"},
		{"dictionary with a container key", "a{asv}", []any{[]DictEntry{}}, "basic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var e encoder
			err := e.values(tc.sig, tc.v)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

// TestATruncatedMessageIsAnErrorAtEveryLength is the property the reader
// goroutine depends on: whatever arrives, readMessage returns an error rather
// than panicking or returning half a message.
func TestATruncatedMessageIsAnErrorAtEveryLength(t *testing.T) {
	for i := range len(helloReply) {
		if _, err := ReadMessage(bytes.NewReader(helloReply[:i])); err == nil {
			t.Fatalf("a message cut after %d of %d bytes was accepted", i, len(helloReply))
		}
	}
}

func TestReadMessageRefusesSizesBeyondTheLimits(t *testing.T) {
	b := bytes.Clone(helloReply)
	binary.LittleEndian.PutUint32(b[4:], maxMessageBytes+1)
	if _, err := ReadMessage(bytes.NewReader(b)); err == nil || !strings.Contains(err.Error(), "size limits") {
		t.Fatalf("a body of more than %d bytes: error %v", maxMessageBytes, err)
	}
}

func TestReadMessageRefusesAHeaderFieldOfTheWrongType(t *testing.T) {
	b := bytes.Clone(helloReply)
	b[18] = 's' // REPLY_SERIAL claimed as a string
	if _, err := ReadMessage(bytes.NewReader(b)); err == nil {
		t.Fatal("a REPLY_SERIAL of type s was accepted")
	}
}

// FuzzReadMessage feeds arbitrary bytes to the reader. Its only assertion is
// the absence of a panic, which is the one failure the reader goroutine
// cannot report.
func FuzzReadMessage(f *testing.F) {
	f.Add(helloReply)
	notifyCall, _ := (&Message{Type: TypeMethodCall, Serial: 1, Path: notifyPath, Member: "Notify",
		Sig: "a{sv}", Body: []any{[]DictEntry{{Key: "k", Value: Variant{Sig: "s", Value: "v"}}}}}).Marshal()
	f.Add(notifyCall)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = ReadMessage(bytes.NewReader(b))
	})
}

func TestReadMessageKeepsTheStreamInStepAfterAnUndecodableMessage(t *testing.T) {
	good, err := (&Message{Type: TypeSignal, Serial: 2, Path: "/a", Interface: "a.b", Member: "C", Sig: "s",
		Body: []any{"next"}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	bad, err := (&Message{Type: TypeMethodCall, Serial: 1, Path: "/a", Member: "M", Sender: ":1.1", Sig: "u",
		Body: []any{uint32(0)}}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(bad, []byte("\x08\x01g\x00\x01u\x00"))
	bad[i+5] = 'h'
	r := bytes.NewReader(append(bad, good...))
	m, err := ReadMessage(r)
	if !errors.Is(err, ErrUndecodable) || m == nil || !m.Header() || m.Sender != ":1.1" || m.Serial != 1 {
		t.Fatalf("message %+v, error %v", m, err)
	}
	m, err = ReadMessage(r)
	if err != nil || m.Body[0] != "next" {
		t.Fatalf("the next message %+v, error %v", m, err)
	}
	// Beyond a receive limit, the header is still decoded.
	r = bytes.NewReader(append(append([]byte{}, good...), good...))
	m, err = readMessage(r, func() int { return len(good) - 1 })
	if !errors.Is(err, ErrUndecodable) || !m.Header() || m.Member != "C" || m.Body != nil {
		t.Fatalf("message %+v, error %v", m, err)
	}
	if m, err = ReadMessage(r); err != nil || m.Body[0] != "next" {
		t.Fatalf("the next message %+v, error %v", m, err)
	}
}
