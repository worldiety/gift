package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

// This file is the D-Bus wire format: the marshalling of values and the
// framing of messages. It is the part of the client that is arithmetic rather
// than syscall, and like clipboard/selection.go it carries **no build tag on
// purpose**: the machine this package is developed on is a Mac, and logic
// behind `//go:build linux` is logic no default `go test ./...` ever
// executes. Here it is covered everywhere, against the byte layouts of the
// specification.
//
// # What it covers
//
// Every type of the specification except UNIX_FD ('h'), which needs
// SCM_RIGHTS on the socket and which neither a notification server nor a
// status notifier host sends. A signature may name it, but a value of it is
// neither written nor read: a message that carries one is refused with
// [ErrUndecodable]. Both byte orders on reading, little endian on
// writing; a peer must accept either and every machine this project targets
// is little endian.
//
// The Go representation of the D-Bus types:
//
//	y byte         b bool        n int16       q uint16
//	i int32        u uint32      x int64       t uint64
//	d float64      s string      o ObjectPath  g Signature
//	v Variant      (…) []any     {kv} DictEntry
//	a… []any, or []string for "as", []byte for "ay" and []DictEntry for
//	   "a{…}" when writing
//
// Reading returns []any for every array except two: one of dictionary
// entries comes back as []DictEntry, and one of bytes as []byte, because a
// picture of 48 by 48 pixels is nine thousand bytes that would otherwise be
// nine thousand interfaces. There is no reflection: the caller states
// the signature, and a value of the wrong Go type is an error rather than a
// conversion.
//
// # Limits
//
// The ones the specification sets, enforced in both directions so that a
// broken peer cannot make this package allocate without bound: 64 MiB for an
// array, 128 MiB for a message, and a nesting depth of 64 containers.

// ObjectPath is the D-Bus type 'o'.
type ObjectPath string

// Signature is the D-Bus type 'g'.
type Signature string

// Variant is the D-Bus type 'v': a value together with its own signature,
// which must be a single complete type.
type Variant struct {
	Sig   Signature
	Value any
}

// DictEntry is the D-Bus type "{kv}", which only exists as the element of an
// array.
type DictEntry struct {
	Key, Value any
}

const (
	maxArrayBytes   = 64 << 20
	maxMessageBytes = 128 << 20
	maxDepth        = 64
	maxSignature    = 255
)

// The message types and header fields of the specification, section "Message
// Format".
const (
	TypeMethodCall   byte = 1
	TypeMethodReturn byte = 2
	TypeError        byte = 3
	TypeSignal       byte = 4

	FlagNoReplyExpected byte = 0x1

	fieldPath        byte = 1
	fieldInterface   byte = 2
	fieldMember      byte = 3
	fieldErrorName   byte = 4
	fieldReplySerial byte = 5
	fieldDestination byte = 6
	fieldSender      byte = 7
	fieldSignature   byte = 8
)

var errTruncated = errors.New("gift/dbus: D-Bus data ends in the middle of a value")

// nextType splits the first single complete type off sig.
func nextType(sig string) (string, string, error) {
	n, err := completeLen(sig, 0)
	if err != nil {
		return "", "", err
	}
	return sig[:n], sig[n:], nil
}

// completeLen returns the length of the single complete type at the start of
// sig.
func completeLen(sig string, depth int) (int, error) {
	if depth > maxDepth {
		return 0, fmt.Errorf("gift/dbus: D-Bus signature nests deeper than %d", maxDepth)
	}
	if sig == "" {
		return 0, errors.New("gift/dbus: D-Bus signature ends where a type was expected")
	}
	switch c := sig[0]; c {
	case 'y', 'b', 'n', 'q', 'i', 'u', 'x', 't', 'd', 's', 'o', 'g', 'v', 'h':
		return 1, nil
	case 'a':
		n, err := completeLen(sig[1:], depth+1)
		return 1 + n, err
	case '(':
		i := 1
		for {
			if i >= len(sig) {
				return 0, fmt.Errorf("gift/dbus: D-Bus signature %q has an unclosed struct", sig)
			}
			if sig[i] == ')' {
				if i == 1 {
					return 0, errors.New("gift/dbus: D-Bus signature has an empty struct")
				}
				return i + 1, nil
			}
			n, err := completeLen(sig[i:], depth+1)
			if err != nil {
				return 0, err
			}
			i += n
		}
	case '{':
		if len(sig) < 2 || !isBasic(sig[1]) {
			return 0, fmt.Errorf("gift/dbus: D-Bus dictionary key in %q is not a basic type", sig)
		}
		n, err := completeLen(sig[2:], depth+1)
		if err != nil {
			return 0, err
		}
		end := 2 + n
		if end >= len(sig) || sig[end] != '}' {
			return 0, fmt.Errorf("gift/dbus: D-Bus dictionary entry in %q is not closed after two types", sig)
		}
		return end + 1, nil
	default:
		return 0, fmt.Errorf("gift/dbus: unsupported D-Bus type code %q", c)
	}
}

func isBasic(c byte) bool {
	return strings.IndexByte("ybnqiuxtdsogh", c) >= 0
}

// validSignature reports whether s is a sequence of complete types that fits
// the 255 byte limit of the type 'g'.
func validSignature(s string) error {
	if len(s) > maxSignature {
		return fmt.Errorf("gift/dbus: D-Bus signature is longer than %d bytes", maxSignature)
	}
	for s != "" {
		_, rest, err := nextType(s)
		if err != nil {
			return err
		}
		s = rest
	}
	return nil
}

// validString checks what the specification requires of every string on the
// wire: valid UTF-8 and no NUL. dbus-daemon disconnects a client that sends
// anything else, so this is checked before a byte is written.
func validString(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("gift/dbus: D-Bus strings must be valid UTF-8")
	}
	if strings.IndexByte(s, 0) >= 0 {
		return errors.New("gift/dbus: D-Bus strings must not contain NUL")
	}
	return nil
}

// validObjectPath checks the grammar of the type 'o': "/" or a sequence of
// "/name" elements of [A-Za-z0-9_], without a trailing slash.
func validObjectPath(p string) error {
	if p == "/" {
		return nil
	}
	if p == "" || p[0] != '/' || p[len(p)-1] == '/' {
		return fmt.Errorf("gift/dbus: %q is not a D-Bus object path", p)
	}
	for _, elem := range strings.Split(p[1:], "/") {
		if elem == "" {
			return fmt.Errorf("gift/dbus: %q is not a D-Bus object path", p)
		}
		for i := 0; i < len(elem); i++ {
			c := elem[i]
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return fmt.Errorf("gift/dbus: %q is not a D-Bus object path", p)
			}
		}
	}
	return nil
}

// alignOf is the alignment of the type that starts with c.
func alignOf(c byte) int {
	switch c {
	case 'n', 'q':
		return 2
	case 'b', 'i', 'u', 's', 'o', 'a', 'h':
		return 4
	case 'x', 't', 'd', '(', '{':
		return 8
	}
	return 1
}

// encoder appends values in little endian. Alignment is relative to the start
// of buf, which is why a body is encoded into its own encoder: the body of a
// message starts on an eight byte boundary, so the two agree.
type encoder struct {
	buf   []byte
	depth int
}

func (e *encoder) pad(n int) {
	for len(e.buf)%n != 0 {
		e.buf = append(e.buf, 0)
	}
}

func (e *encoder) u32(v uint32) {
	e.pad(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *encoder) u64(v uint64) {
	e.pad(8)
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

func (e *encoder) str(s string) error {
	if err := validString(s); err != nil {
		return err
	}
	e.u32(uint32(len(s)))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
	return nil
}

func (e *encoder) sig(s string) error {
	if err := validSignature(s); err != nil {
		return err
	}
	e.buf = append(e.buf, byte(len(s)))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
	return nil
}

// values encodes one value per complete type of sig.
func (e *encoder) values(sig string, vs []any) error {
	for i, v := range vs {
		if sig == "" {
			return fmt.Errorf("gift/dbus: D-Bus signature has fewer types than the %d values", len(vs))
		}
		t, rest, err := nextType(sig)
		if err != nil {
			return err
		}
		if err := e.value(t, v); err != nil {
			return fmt.Errorf("value %d: %w", i, err)
		}
		sig = rest
	}
	if sig != "" {
		return fmt.Errorf("gift/dbus: D-Bus signature has more types than the %d values", len(vs))
	}
	return nil
}

func mismatch(t string, v any) error {
	return fmt.Errorf("gift/dbus: D-Bus type %q cannot be written from a %T", t, v)
}

// value encodes v as the single complete type t.
func (e *encoder) value(t string, v any) error {
	switch t[0] {
	case 'y':
		x, ok := v.(byte)
		if !ok {
			return mismatch(t, v)
		}
		e.buf = append(e.buf, x)
	case 'b':
		x, ok := v.(bool)
		if !ok {
			return mismatch(t, v)
		}
		var u uint32
		if x {
			u = 1
		}
		e.u32(u)
	case 'n':
		x, ok := v.(int16)
		if !ok {
			return mismatch(t, v)
		}
		e.pad(2)
		e.buf = binary.LittleEndian.AppendUint16(e.buf, uint16(x))
	case 'q':
		x, ok := v.(uint16)
		if !ok {
			return mismatch(t, v)
		}
		e.pad(2)
		e.buf = binary.LittleEndian.AppendUint16(e.buf, x)
	case 'i':
		x, ok := v.(int32)
		if !ok {
			return mismatch(t, v)
		}
		e.u32(uint32(x))
	case 'u':
		x, ok := v.(uint32)
		if !ok {
			return mismatch(t, v)
		}
		e.u32(x)
	case 'x':
		x, ok := v.(int64)
		if !ok {
			return mismatch(t, v)
		}
		e.u64(uint64(x))
	case 't':
		x, ok := v.(uint64)
		if !ok {
			return mismatch(t, v)
		}
		e.u64(x)
	case 'd':
		x, ok := v.(float64)
		if !ok {
			return mismatch(t, v)
		}
		e.u64(math.Float64bits(x))
	case 's':
		x, ok := v.(string)
		if !ok {
			return mismatch(t, v)
		}
		return e.str(x)
	case 'o':
		x, ok := v.(ObjectPath)
		if !ok {
			return mismatch(t, v)
		}
		if err := validObjectPath(string(x)); err != nil {
			return err
		}
		return e.str(string(x))
	case 'g':
		x, ok := v.(Signature)
		if !ok {
			return mismatch(t, v)
		}
		return e.sig(string(x))
	case 'v':
		x, ok := v.(Variant)
		if !ok {
			return mismatch(t, v)
		}
		inner, rest, err := nextType(string(x.Sig))
		if err != nil || rest != "" {
			return fmt.Errorf("gift/dbus: variant signature %q is not a single complete type", x.Sig)
		}
		if err := e.sig(inner); err != nil {
			return err
		}
		return e.nested(func() error { return e.value(inner, x.Value) })
	case 'a':
		return e.nested(func() error { return e.array(t, v) })
	case '(':
		x, ok := v.([]any)
		if !ok {
			return mismatch(t, v)
		}
		e.pad(8)
		return e.nested(func() error { return e.values(t[1:len(t)-1], x) })
	case '{':
		x, ok := v.(DictEntry)
		if !ok {
			return mismatch(t, v)
		}
		e.pad(8)
		key, val := t[1:2], t[2:len(t)-1]
		return e.nested(func() error { return e.values(key+val, []any{x.Key, x.Value}) })
	default:
		return fmt.Errorf("gift/dbus: unsupported D-Bus type %q", t)
	}
	return nil
}

func (e *encoder) nested(fn func() error) error {
	e.depth++
	defer func() { e.depth-- }()
	if e.depth > maxDepth {
		return fmt.Errorf("gift/dbus: D-Bus value nests deeper than %d", maxDepth)
	}
	return fn()
}

// array encodes the array type t ("a…"). The length counts the elements only,
// not the padding between the length and the first element.
func (e *encoder) array(t string, v any) error {
	elem := t[1:]
	e.u32(0)
	at := len(e.buf) - 4
	e.pad(alignOf(elem[0]))
	start := len(e.buf)
	switch xs := v.(type) {
	case []byte:
		if elem != "y" {
			return mismatch(t, v)
		}
		e.buf = append(e.buf, xs...)
	case []string:
		if elem != "s" {
			return mismatch(t, v)
		}
		for _, x := range xs {
			if err := e.str(x); err != nil {
				return err
			}
		}
	case []DictEntry:
		if elem[0] != '{' {
			return mismatch(t, v)
		}
		for _, x := range xs {
			if err := e.value(elem, x); err != nil {
				return err
			}
		}
	case []any:
		for _, x := range xs {
			if err := e.value(elem, x); err != nil {
				return err
			}
		}
	default:
		return mismatch(t, v)
	}
	n := len(e.buf) - start
	if n > maxArrayBytes {
		return fmt.Errorf("gift/dbus: D-Bus array of %d bytes exceeds the limit of %d", n, maxArrayBytes)
	}
	binary.LittleEndian.PutUint32(e.buf[at:], uint32(n))
	return nil
}

// decoder reads values from buf, which starts at the start of a message, so
// that alignment is computed the way the sender computed it.
type decoder struct {
	buf   []byte
	off   int
	order binary.ByteOrder
	depth int
}

func (d *decoder) align(n int) error {
	p := (n - d.off%n) % n
	if d.off+p > len(d.buf) {
		return errTruncated
	}
	for _, b := range d.buf[d.off : d.off+p] {
		if b != 0 {
			return errors.New("gift/dbus: D-Bus padding is not zero")
		}
	}
	d.off += p
	return nil
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || d.off+n > len(d.buf) {
		return nil, errTruncated
	}
	b := d.buf[d.off : d.off+n]
	d.off += n
	return b, nil
}

func (d *decoder) u32() (uint32, error) {
	if err := d.align(4); err != nil {
		return 0, err
	}
	b, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return d.order.Uint32(b), nil
}

func (d *decoder) u64() (uint64, error) {
	if err := d.align(8); err != nil {
		return 0, err
	}
	b, err := d.take(8)
	if err != nil {
		return 0, err
	}
	return d.order.Uint64(b), nil
}

func (d *decoder) str() (string, error) {
	n, err := d.u32()
	if err != nil {
		return "", err
	}
	if n > maxMessageBytes {
		return "", errTruncated
	}
	b, err := d.take(int(n) + 1)
	if err != nil {
		return "", err
	}
	if b[n] != 0 {
		return "", errors.New("gift/dbus: D-Bus string is not NUL terminated")
	}
	s := string(b[:n])
	if err := validString(s); err != nil {
		return "", err
	}
	return s, nil
}

func (d *decoder) sig() (string, error) {
	b, err := d.take(1)
	if err != nil {
		return "", err
	}
	n := int(b[0])
	b, err = d.take(n + 1)
	if err != nil {
		return "", err
	}
	if b[n] != 0 {
		return "", errors.New("gift/dbus: D-Bus signature is not NUL terminated")
	}
	s := string(b[:n])
	if err := validSignature(s); err != nil {
		return "", err
	}
	return s, nil
}

// values decodes one value per complete type of sig.
func (d *decoder) values(sig string) ([]any, error) {
	var out []any
	for sig != "" {
		t, rest, err := nextType(sig)
		if err != nil {
			return nil, err
		}
		v, err := d.value(t)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		sig = rest
	}
	return out, nil
}

// value decodes the single complete type t.
func (d *decoder) value(t string) (any, error) {
	switch t[0] {
	case 'y':
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return b[0], nil
	case 'b':
		u, err := d.u32()
		if err != nil {
			return nil, err
		}
		if u > 1 {
			return nil, fmt.Errorf("gift/dbus: D-Bus boolean holds %d", u)
		}
		return u == 1, nil
	case 'n', 'q':
		if err := d.align(2); err != nil {
			return nil, err
		}
		b, err := d.take(2)
		if err != nil {
			return nil, err
		}
		u := d.order.Uint16(b)
		if t[0] == 'n' {
			return int16(u), nil
		}
		return u, nil
	case 'i':
		u, err := d.u32()
		return int32(u), err
	case 'u':
		return d.u32()
	case 'h':
		// A signature may name it, so that a message with one is framed
		// and refused rather than ending the connection; the value is an
		// index into file descriptors this package never negotiates.
		return nil, errors.New("gift/dbus: D-Bus UNIX_FD values are not supported")
	case 'x':
		u, err := d.u64()
		return int64(u), err
	case 't':
		return d.u64()
	case 'd':
		u, err := d.u64()
		return math.Float64frombits(u), err
	case 's':
		return d.str()
	case 'o':
		s, err := d.str()
		if err != nil {
			return nil, err
		}
		if err := validObjectPath(s); err != nil {
			return nil, err
		}
		return ObjectPath(s), nil
	case 'g':
		s, err := d.sig()
		return Signature(s), err
	case 'v':
		s, err := d.sig()
		if err != nil {
			return nil, err
		}
		inner, rest, err := nextType(s)
		if err != nil || rest != "" {
			return nil, fmt.Errorf("gift/dbus: variant signature %q is not a single complete type", s)
		}
		var v any
		err = d.nested(func() (err error) { v, err = d.value(inner); return })
		return Variant{Sig: Signature(inner), Value: v}, err
	case 'a':
		var v any
		err := d.nested(func() (err error) { v, err = d.array(t[1:]); return })
		return v, err
	case '(':
		if err := d.align(8); err != nil {
			return nil, err
		}
		var v []any
		err := d.nested(func() (err error) { v, err = d.values(t[1 : len(t)-1]); return })
		return v, err
	case '{':
		if err := d.align(8); err != nil {
			return nil, err
		}
		var kv []any
		err := d.nested(func() (err error) { kv, err = d.values(t[1 : len(t)-1]); return })
		if err != nil {
			return nil, err
		}
		return DictEntry{Key: kv[0], Value: kv[1]}, nil
	}
	return nil, fmt.Errorf("gift/dbus: unsupported D-Bus type %q", t)
}

func (d *decoder) nested(fn func() error) error {
	d.depth++
	defer func() { d.depth-- }()
	if d.depth > maxDepth {
		return fmt.Errorf("gift/dbus: D-Bus value nests deeper than %d", maxDepth)
	}
	return fn()
}

func (d *decoder) array(elem string) (any, error) {
	n, err := d.u32()
	if err != nil {
		return nil, err
	}
	if n > maxArrayBytes {
		return nil, fmt.Errorf("gift/dbus: D-Bus array of %d bytes exceeds the limit of %d", n, maxArrayBytes)
	}
	if err := d.align(alignOf(elem[0])); err != nil {
		return nil, err
	}
	end := d.off + int(n)
	if end > len(d.buf) {
		return nil, errTruncated
	}
	if elem == "y" {
		out := bytes.Clone(d.buf[d.off:end])
		d.off = end
		return out, nil
	}
	if elem[0] == '{' {
		var out []DictEntry
		for d.off < end {
			v, err := d.value(elem)
			if err != nil {
				return nil, err
			}
			out = append(out, v.(DictEntry))
		}
		if d.off != end {
			return nil, errors.New("gift/dbus: D-Bus array length does not match its elements")
		}
		return out, nil
	}
	var out []any
	for d.off < end {
		v, err := d.value(elem)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if d.off != end {
		return nil, errors.New("gift/dbus: D-Bus array length does not match its elements")
	}
	return out, nil
}

// Message is one D-Bus message, with the header fields this package reads or
// writes spelled out. Unknown header fields are ignored on reading, as the
// specification requires.
type Message struct {
	Type        byte
	Flags       byte
	Serial      uint32
	Path        ObjectPath
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Sig         Signature
	Body        []any

	// header says that ReadMessage decoded the header fields; see
	// [Message.Header].
	header bool
}

// Marshal encodes m in little endian.
func (m *Message) Marshal() ([]byte, error) {
	var body encoder
	if err := body.values(string(m.Sig), m.Body); err != nil {
		return nil, err
	}

	var fields []any
	add := func(code byte, sig Signature, v any) {
		fields = append(fields, []any{code, Variant{Sig: sig, Value: v}})
	}
	if m.Path != "" {
		add(fieldPath, "o", m.Path)
	}
	if m.Interface != "" {
		add(fieldInterface, "s", m.Interface)
	}
	if m.Member != "" {
		add(fieldMember, "s", m.Member)
	}
	if m.ErrorName != "" {
		add(fieldErrorName, "s", m.ErrorName)
	}
	if m.ReplySerial != 0 {
		add(fieldReplySerial, "u", m.ReplySerial)
	}
	if m.Destination != "" {
		add(fieldDestination, "s", m.Destination)
	}
	if m.Sender != "" {
		add(fieldSender, "s", m.Sender)
	}
	if m.Sig != "" {
		add(fieldSignature, "g", m.Sig)
	}

	e := encoder{buf: make([]byte, 0, 128+len(body.buf))}
	e.buf = append(e.buf, 'l', m.Type, m.Flags, 1)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(len(body.buf)))
	e.buf = binary.LittleEndian.AppendUint32(e.buf, m.Serial)
	if err := e.value("a(yv)", fields); err != nil {
		return nil, err
	}
	e.pad(8)
	e.buf = append(e.buf, body.buf...)
	if len(e.buf) > maxMessageBytes {
		return nil, fmt.Errorf("gift/dbus: D-Bus message of %d bytes exceeds the limit of %d", len(e.buf), maxMessageBytes)
	}
	return e.buf, nil
}

// ErrUndecodable is wrapped by the error of [ReadMessage] for a message
// that was read whole but cannot be used: its header fields or its body do
// not decode, it is larger than the receive limit of the connection, or its
// serial is 0. The stream is still in step, the next message can be read,
// and the [Message] returned along with the error holds what was decoded:
// Type, Flags and Serial always, the header fields when [Message.Header]
// says so.
//
// A peer can send such a message on purpose — dbus-daemon routes a body
// with a UNIX_FD in its signature to a connection that negotiated no file
// descriptors, which this package refuses to decode — so a connection skips
// it rather than closing. Only a message that cannot be framed, of an
// unknown byte order or protocol, or beyond the limits of the
// specification, ends the stream.
var ErrUndecodable = errors.New("gift/dbus: the D-Bus message cannot be decoded")

// ReadMessage reads exactly one message from r. An error that wraps
// [ErrUndecodable] comes with the message as far as it was decoded; every
// other error comes with nil and means that the stream is lost.
func ReadMessage(r io.Reader) (*Message, error) {
	return readMessage(r, func() int { return maxMessageBytes })
}

// readMessage is [ReadMessage] with a receive limit: a message longer than
// limit bytes is read past and returned undecoded, with its header fields if
// they are within the limit. limit is asked once the message has begun to
// arrive, so that a limit set while the reader waits applies to the message
// it waits for.
func readMessage(r io.Reader, limitf func() int) (*Message, error) {
	var head [16]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	limit := limitf()
	var order binary.ByteOrder
	switch head[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("gift/dbus: D-Bus message has the unknown byte order %q", head[0])
	}
	if head[3] != 1 {
		return nil, fmt.Errorf("gift/dbus: D-Bus message has protocol version %d", head[3])
	}
	bodyLen := order.Uint32(head[4:])
	fieldsLen := order.Uint32(head[12:])
	if fieldsLen > maxArrayBytes || bodyLen > maxMessageBytes {
		return nil, errors.New("gift/dbus: D-Bus message exceeds the size limits")
	}
	headerEnd := 16 + int(fieldsLen)
	bodyStart := (headerEnd + 7) &^ 7
	total := bodyStart + int(bodyLen)
	if total > maxMessageBytes {
		return nil, errors.New("gift/dbus: D-Bus message exceeds the size limits")
	}
	m := &Message{Type: head[1], Flags: head[2], Serial: order.Uint32(head[8:])}

	// skip reads past n bytes, for a message that is not decoded whole.
	skip := func(n int) error {
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		return nil
	}
	read := total
	if total > limit {
		read = bodyStart
		if bodyStart > limit {
			if err := skip(total - 16); err != nil {
				return nil, err
			}
			return m, fmt.Errorf("%w: %d bytes exceed the receive limit of %d", ErrUndecodable, total, limit)
		}
	}
	buf := make([]byte, read)
	copy(buf, head[:])
	if _, err := io.ReadFull(r, buf[16:]); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if read < total {
		if err := skip(total - read); err != nil {
			return nil, err
		}
	}
	undecodable := func(err error) (*Message, error) {
		return m, fmt.Errorf("%w: %w", ErrUndecodable, err)
	}

	if m.Serial == 0 {
		return undecodable(errors.New("gift/dbus: D-Bus message has serial 0"))
	}
	if err := m.decodeHeader(buf[:headerEnd], order); err != nil {
		*m = Message{Type: m.Type, Flags: m.Flags, Serial: m.Serial}
		return undecodable(err)
	}
	m.header = true
	if read < total {
		return undecodable(fmt.Errorf("gift/dbus: D-Bus message of %d bytes exceeds the receive limit of %d", total, limit))
	}

	d := decoder{buf: buf, off: headerEnd, order: order}
	var err error
	if err = d.align(8); err != nil {
		return undecodable(err)
	}
	if m.Body, err = d.values(string(m.Sig)); err != nil {
		m.Body = nil
		return undecodable(fmt.Errorf("gift/dbus: D-Bus body of %s: %w", m.Member, err))
	}
	if d.off != total {
		m.Body = nil
		return undecodable(errors.New("gift/dbus: D-Bus body length does not match its signature"))
	}
	return m, nil
}

// decodeHeader decodes the header fields of the message in buf, which ends
// where they end.
func (m *Message) decodeHeader(buf []byte, order binary.ByteOrder) error {
	d := decoder{buf: buf, off: 12, order: order}
	raw, err := d.value("a(yv)")
	if err != nil {
		return fmt.Errorf("gift/dbus: D-Bus header: %w", err)
	}
	for _, f := range raw.([]any) {
		f := f.([]any)
		code, v := f[0].(byte), f[1].(Variant)
		if err := m.setField(code, v); err != nil {
			return err
		}
	}
	if d.off != len(buf) {
		return errors.New("gift/dbus: D-Bus header length does not match its fields")
	}
	return nil
}

// Header reports whether the header fields of a message that [ReadMessage]
// returned with [ErrUndecodable] were decoded: Path, Member, Sender,
// ReplySerial and the others. It is true for every message read without an
// error.
func (m *Message) Header() bool { return m.header }

// setField stores one header field, checking that its variant has the type
// the specification assigns to the code.
func (m *Message) setField(code byte, v Variant) error {
	want := map[byte]Signature{
		fieldPath: "o", fieldInterface: "s", fieldMember: "s", fieldErrorName: "s",
		fieldReplySerial: "u", fieldDestination: "s", fieldSender: "s", fieldSignature: "g",
	}[code]
	if want == "" {
		return nil
	}
	if v.Sig != want {
		return fmt.Errorf("gift/dbus: D-Bus header field %d has type %q, want %q", code, v.Sig, want)
	}
	switch code {
	case fieldPath:
		m.Path = v.Value.(ObjectPath)
	case fieldInterface:
		m.Interface = v.Value.(string)
	case fieldMember:
		m.Member = v.Value.(string)
	case fieldErrorName:
		m.ErrorName = v.Value.(string)
	case fieldReplySerial:
		m.ReplySerial = v.Value.(uint32)
	case fieldDestination:
		m.Destination = v.Value.(string)
	case fieldSender:
		m.Sender = v.Value.(string)
	case fieldSignature:
		m.Sig = v.Value.(Signature)
	}
	return nil
}
