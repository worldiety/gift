package notify

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is the part of the Windows half that is text and arithmetic: the
// toast XML, the check of the AppUserModelID and the derivation of the COM
// class id. Like wire.go it carries no build tag on purpose, so that it is
// tested on the machines this project is developed on, none of which runs
// Windows. The calls into the Windows Runtime are in platform_windows.go.

// maxToastBytes bounds the XML of one toast. Windows refuses a payload over
// 5 KB; the margin below it is for the attribute and element names, which
// are counted in, and for an interpretation of "KB" as a thousand.
const maxToastBytes = 4900

// maxAUMID is the longest AppUserModelID Windows accepts. A longer one makes
// scheduled toasts fail with ERROR_INSUFFICIENT_BUFFER, and the shell's own
// documentation sets 128 characters as the limit.
const maxAUMID = 128

// checkAUMID reports whether id can serve as an AppUserModelID and as the
// name of the registry key under HKCU\Software\Classes\AppUserModelId. A
// backslash would make it a path of keys, a space is outside the convention
// Company.Product.SubProduct, and an empty one is not an identity at all.
func checkAUMID(id string) error {
	switch {
	case id == "":
		return fmt.Errorf("%w: Config.AppID is required on Windows, for example com.example.Photos", ErrUnsupported)
	case len(id) > maxAUMID:
		return fmt.Errorf("%w: Config.AppID %q is longer than %d characters", ErrUnsupported, id, maxAUMID)
	case strings.ContainsAny(id, "\\ \t\r\n") || !utf8.ValidString(id):
		return fmt.Errorf("%w: Config.AppID %q contains a backslash, white space or invalid UTF-8", ErrUnsupported, id)
	}
	return nil
}

// activatorCLSID derives the class id of the application's notification
// activator from its AppUserModelID: a name based UUID, version 5, so that
// one application gets the same id in every run and two applications never
// share one. The id goes into the registry as CustomActivator and is the
// class this process registers with CoRegisterClassObject.
func activatorCLSID(aumid string) [16]byte {
	sum := sha1.Sum([]byte("github.com/worldiety/gift/notify:" + aumid))
	var u [16]byte
	copy(u[:], sum[:16])
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return u
}

// guidString formats u the way the registry spells a CLSID, with braces, in
// the field order of a GUID: the first three fields are integers, which is
// what Windows reads them as, and they are written most significant first.
func guidString(u [16]byte) string {
	return fmt.Sprintf("{%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		u[0], u[1], u[2], u[3], u[4], u[5], u[6], u[7], u[8], u[9], u[10], u[11], u[12], u[13], u[14], u[15])
}

// toastXML builds the content of a toast: the generic template with the title
// and, if there is one, the body. launch comes back to the activator when the
// toast is clicked.
//
// What does not fit into [maxToastBytes] is cut from the body first and then
// from the title, at a rune boundary and before escaping, so that an entity
// is never cut in half. The result reports whether it had to cut.
func toastXML(launch, title, body string) (string, bool) {
	title, body = xmlText(title), xmlText(body)
	build := func(t, b string) string {
		var s strings.Builder
		s.WriteString(`<toast launch="`)
		s.WriteString(xmlEscape(launch))
		s.WriteString(`"><visual><binding template="ToastGeneric"><text>`)
		s.WriteString(xmlEscape(t))
		s.WriteString(`</text>`)
		if b != "" {
			s.WriteString(`<text>`)
			s.WriteString(xmlEscape(b))
			s.WriteString(`</text>`)
		}
		s.WriteString(`</binding></visual></toast>`)
		return s.String()
	}
	x := build(title, body)
	if len(x) <= maxToastBytes {
		return x, false
	}
	// Cut the body rune by rune from the end until it fits; then, if
	// the title alone is too long, the title. Quadratic in the excess, which
	// is bounded by MaxBytes and only paid by a notification that is too long.
	for body != "" && len(x) > maxToastBytes {
		_, n := utf8.DecodeLastRuneInString(body)
		body = body[:len(body)-n]
		x = build(title, body)
	}
	for title != "" && len(x) > maxToastBytes {
		_, n := utf8.DecodeLastRuneInString(title)
		title = title[:len(title)-n]
		x = build(title, body)
	}
	return x, true
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")

func xmlEscape(s string) string { return xmlEscaper.Replace(s) }

// xmlText replaces what XML 1.0 does not allow in a document — the control
// characters other than tab, line feed and carriage return — with a space.
// LoadXml refuses a document that contains one, and the user's text should
// not decide whether a notification appears.
func xmlText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xfffe || r == 0xffff {
			return ' '
		}
		return r
	}, s)
}
