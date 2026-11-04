package email

import (
	"errors"
	"net/mail"
	"strings"
)

// ErrInvalidAddress is returned when a recipient address fails formal validation.
var ErrInvalidAddress = errors.New("invalid email address")

// maxAddressLen is the maximum length of a forward-path address (RFC 5321).
const maxAddressLen = 254

// ValidateAddress performs a formal check on a single recipient address.
// Only a bare addr-spec is accepted (no display name, comments or lists),
// the domain must contain at least one dot, and CR/LF are rejected so the
// value can be written safely into the To: header.
func ValidateAddress(s string) error {
	if s == "" || len(s) > maxAddressLen || strings.ContainsAny(s, "\r\n") {
		return ErrInvalidAddress
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" || addr.Address != s {
		return ErrInvalidAddress
	}
	at := strings.LastIndexByte(s, '@')
	if at <= 0 {
		return ErrInvalidAddress
	}
	domain := s[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return ErrInvalidAddress
	}
	return nil
}
