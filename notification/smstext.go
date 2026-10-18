package notification

import (
	"strings"
	"unicode/utf8"
)

// SMS sizes. A single SMS holds 160 GSM-7 characters or 70 UCS-2 code units;
// a message split into several segments loses room to the joining header, so
// each segment then holds 153 or 67.
const (
	gsmSingle     = 160
	gsmSegment    = 153
	unicodeSingle = 70
	unicodeMulti  = 67
)

// gsmBasic and gsmExtended are the GSM 03.38 alphabet. Extended characters
// take two positions. One character outside both switches the whole message
// to UCS-2, which cuts its room by more than half; Spanish á, í, ó, ú do that.
const (
	gsmBasic    = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
	gsmExtended = "^{}\\[~]|€\f"
)

// isGSM reports whether text fits the GSM-7 alphabet.
func isGSM(text string) bool {
	for _, r := range text {
		if !strings.ContainsRune(gsmBasic, r) && !strings.ContainsRune(gsmExtended, r) {
			return false
		}
	}
	return true
}

// smsLength is text's length in the units its encoding counts: GSM-7
// positions, or UCS-2 code units.
func smsLength(text string, gsm bool) int {
	n := 0
	for _, r := range text {
		switch {
		case gsm && strings.ContainsRune(gsmExtended, r):
			n += 2
		case !gsm && r > 0xFFFF:
			n += 2 // a surrogate pair, e.g. an emoji
		default:
			n++
		}
	}
	return n
}

// smsSegments is how many SMS segments text takes.
func smsSegments(text string) int {
	gsm := isGSM(text)
	n := smsLength(text, gsm)
	single, multi := gsmSingle, gsmSegment
	if !gsm {
		single, multi = unicodeSingle, unicodeMulti
	}
	if n <= single {
		return 1
	}
	return (n + multi - 1) / multi
}

// smsCapacity is how much text fits in maxSegments segments.
func smsCapacity(maxSegments int, gsm bool) int {
	if gsm {
		if maxSegments <= 1 {
			return gsmSingle
		}
		return maxSegments * gsmSegment
	}
	if maxSegments <= 1 {
		return unicodeSingle
	}
	return maxSegments * unicodeMulti
}

// composeSMS builds the message: title, body and link on their own lines,
// with the body (then the title) shortened to fit maxSegments. The link is
// never cut. It returns the text and whether it needs UCS-2.
func composeSMS(title, body, link string, maxSegments int) (string, bool) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	join := func(title, body string) string {
		var parts []string
		for _, p := range []string{title, body, link} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, "\n")
	}

	full := join(title, body)
	gsm := isGSM(full)
	capacity := smsCapacity(maxSegments, gsm)
	if smsLength(full, gsm) <= capacity {
		return full, !gsm
	}

	// "…" is not GSM-7: using it would switch the message to UCS-2.
	ellipsis := "..."
	if !gsm {
		ellipsis = "…"
	}
	shorten := func(s string, drop int) string {
		runes := []rune(s)
		for len(runes) > 0 && drop > 0 {
			drop -= smsLength(string(runes[len(runes)-1]), gsm)
			runes = runes[:len(runes)-1]
		}
		return strings.TrimSpace(string(runes)) + ellipsis
	}

	over := smsLength(full, gsm) - capacity + smsLength(ellipsis, gsm)
	if bodyLen := smsLength(body, gsm); bodyLen > over {
		return join(title, shorten(body, over)), !gsm
	}
	// The body alone is not enough: drop it and shorten the title.
	withoutBody := join(title, "")
	over = smsLength(withoutBody, gsm) - capacity + smsLength(ellipsis, gsm)
	if over > 0 && utf8.RuneCountInString(title) > 0 {
		title = shorten(title, over)
	}
	return join(title, ""), !gsm
}
