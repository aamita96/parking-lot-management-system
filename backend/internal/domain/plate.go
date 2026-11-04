package domain

import (
	"net/mail"
	"regexp"
	"strings"
)

var (
	standardPlate = regexp.MustCompile(`^[A-Z]{2}[0-9]{1,2}[A-Z]{0,3}[0-9]{4}$`)
	bhPlate       = regexp.MustCompile(`^[0-9]{2}BH[0-9]{4}[A-Z]{1,2}$`)
	indianMobile  = regexp.MustCompile(`^[6-9][0-9]{9}$`)
	registrationI = regexp.MustCompile(`^[A-Za-z0-9-]{1,50}$`)
)

// NormalisePlate trims, upper-cases and removes spaces and hyphens (FR-2):
// "ka 01-ab 1234" -> "KA01AB1234".
func NormalisePlate(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		if r == ' ' || r == '-' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ValidPlate reports whether a normalised number matches the standard or BH-series format (FR-3).
func ValidPlate(normalised string) bool {
	return standardPlate.MatchString(normalised) || bhPlate.MatchString(normalised)
}

// NormaliseEmail lower-cases and trims an email address.
func NormaliseEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ValidEmail checks a normalised email: one bare address, max 254 characters, with a dotted domain.
func ValidEmail(e string) bool {
	if e == "" || len(e) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return false
	}
	at := strings.LastIndexByte(e, '@')
	return at > 0 && strings.Contains(e[at+1:], ".")
}

// NormaliseMobile accepts a 10-digit Indian mobile with optional +91 / 91 / 0 prefix
// and returns it in E.164 (+919876543210). ok is false for anything else (FR-3).
func NormaliseMobile(raw string) (e164 string, ok bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if r == ' ' || r == '-' || r == '(' || r == ')' {
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	switch {
	case strings.HasPrefix(s, "+91"):
		s = s[3:]
	case len(s) == 12 && strings.HasPrefix(s, "91"):
		s = s[2:]
	case len(s) == 11 && strings.HasPrefix(s, "0"):
		s = s[1:]
	}
	if !indianMobile.MatchString(s) {
		return "", false
	}
	return "+91" + s, true
}

// ValidRegistrationID checks the optional RC number: up to 50 letters, digits and hyphens.
func ValidRegistrationID(s string) bool {
	return registrationI.MatchString(s)
}
