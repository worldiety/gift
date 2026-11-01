package notify

import (
	"bytes"
	"encoding/binary"
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
	m := &message{typ: msgMethodReturn, serial: 2, replySerial: 1, sig: "s", body: []any{":1.42"}}
	got, err := m.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, helloReply) {
		t.Fatalf("marshal\n got % x\nwant % x", got, helloReply)
	}
}

func TestReadMessageDecodesTheLayoutOfTheSpecification(t *testing.T) {
	m, err := readMessage(bytes.NewReader(helloReply))
	if err != nil {
		t.Fatal(err)
	}
	if m.typ != msgMethodReturn || m.serial != 2 || m.replySerial != 1 || m.sig != "s" {
		t.Fatalf("header %+v", m)
	}
	if len(m.body) != 1 || m.body[0] != ":1.42" {
		t.Fatalf("body %#v", m.body)
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
	m, err := readMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.serial != 2 || m.replySerial != 1 || len(m.body) != 1 || m.body[0] != ":1.42" {
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

// TestTheNotifyCallRoundTrips sends the one call this package exists for, with
// every argument type it uses, through the encoder and back.
func TestTheNotifyCallRoundTrips(t *testing.T) {
	m := &message{
		typ: msgMethodCall, serial: 7, path: notifyPath, iface: notifyInterface, member: "Notify",
		destination: notifyName, sig: "susssasa{sv}i",
		body: []any{"Photos", uint32(3), "", "Import finished", "1 024 pictures",
			[]string{"default", ""},
			[]dictEntry{
				{key: "desktop-entry", value: variant{sig: "s", value: "com.example.Photos"}},
				{key: "urgency", value: variant{sig: "y", value: byte(1)}},
				{key: "resident", value: variant{sig: "b", value: true}},
			},
			int32(-1)},
	}
	b, err := m.marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := readMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"Photos", uint32(3), "", "Import finished", "1 024 pictures",
		[]any{"default", ""},
		[]dictEntry{
			{key: "desktop-entry", value: variant{sig: "s", value: "com.example.Photos"}},
			{key: "urgency", value: variant{sig: "y", value: byte(1)}},
			{key: "resident", value: variant{sig: "b", value: true}},
		},
		int32(-1)}
	if !reflect.DeepEqual(got.body, want) {
		t.Fatalf("body\n got %#v\nwant %#v", got.body, want)
	}
	if got.path != notifyPath || got.iface != notifyInterface || got.member != "Notify" ||
		got.destination != notifyName || got.serial != 7 || got.sig != m.sig {
		t.Fatalf("header %+v", got)
	}
}

func TestEveryBasicTypeRoundTrips(t *testing.T) {
	sig := signature("ybnqiuxtdsogv(us)")
	in := []any{byte(0xfe), true, int16(-2), uint16(0xfffe), int32(-3), uint32(0xfffffffd),
		int64(-4), uint64(0xfffffffffffffffc), 2.5, "s", objectPath("/a/b_1"), signature("a{sv}"),
		variant{sig: "as", value: []string{"x"}}, []any{uint32(9), "nine"}}
	b, err := (&message{typ: msgSignal, serial: 1, path: "/", iface: "a.b", member: "C", sig: sig, body: in}).marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := readMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	in[12] = variant{sig: "as", value: []any{"x"}} // arrays decode as []any
	if !reflect.DeepEqual(m.body, in) {
		t.Fatalf("body\n got %#v\nwant %#v", m.body, in)
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
		{"object path with trailing slash", "o", []any{objectPath("/a/")}, "object path"},
		{"object path with a dash", "o", []any{objectPath("/a-b")}, "object path"},
		{"wrong Go type", "u", []any{int32(1)}, "cannot be written"},
		{"variant of two types", "v", []any{variant{sig: "ss", value: "x"}}, "single complete type"},
		{"too few values", "ss", []any{"x"}, "more types"},
		{"too many values", "s", []any{"x", "y"}, "fewer types"},
		{"unclosed struct", "(s", []any{[]any{"x"}}, "unclosed"},
		{"dictionary with a container key", "a{asv}", []any{[]dictEntry{}}, "basic"},
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
		if _, err := readMessage(bytes.NewReader(helloReply[:i])); err == nil {
			t.Fatalf("a message cut after %d of %d bytes was accepted", i, len(helloReply))
		}
	}
}

func TestReadMessageRefusesSizesBeyondTheLimits(t *testing.T) {
	b := bytes.Clone(helloReply)
	binary.LittleEndian.PutUint32(b[4:], maxMessageBytes+1)
	if _, err := readMessage(bytes.NewReader(b)); err == nil || !strings.Contains(err.Error(), "size limits") {
		t.Fatalf("a body of more than %d bytes: error %v", maxMessageBytes, err)
	}
}

func TestReadMessageRefusesAHeaderFieldOfTheWrongType(t *testing.T) {
	b := bytes.Clone(helloReply)
	b[18] = 's' // REPLY_SERIAL claimed as a string
	if _, err := readMessage(bytes.NewReader(b)); err == nil {
		t.Fatal("a REPLY_SERIAL of type s was accepted")
	}
}

// FuzzReadMessage feeds arbitrary bytes to the reader. Its only assertion is
// the absence of a panic, which is the one failure the reader goroutine
// cannot report.
func FuzzReadMessage(f *testing.F) {
	f.Add(helloReply)
	notifyCall, _ := (&message{typ: msgMethodCall, serial: 1, path: notifyPath, member: "Notify",
		sig: "a{sv}", body: []any{[]dictEntry{{key: "k", value: variant{sig: "s", value: "v"}}}}}).marshal()
	f.Add(notifyCall)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = readMessage(bytes.NewReader(b))
	})
}
