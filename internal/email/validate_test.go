package email

import (
	"strings"
	"testing"
)

func TestValidateAddress(t *testing.T) {
	valid := []string{
		"mario.rossi@example.com",
		"a@b.it",
		"first+tag@sub.example.co.uk",
		"user_name-1@example-domain.org",
	}
	for _, s := range valid {
		if err := ValidateAddress(s); err != nil {
			t.Errorf("ValidateAddress(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"",
		"plainaddress",
		"@example.com",
		"user@",
		"user@localhost",    // dominio senza punto
		"user@example.",     // punto finale
		"user@.example.com", // punto iniziale
		"user@exa mple.com", // spazio
		"user@@example.com",
		"Mario <mario@example.com>", // display name
		"mario@example.com (comment)",
		"a@b.it,c@d.it", // destinatari multipli
		"a@b.it\r\nBcc: victim@evil.com",
		"a@b.it\nBcc: victim@evil.com",
		" a@b.it",
		strings.Repeat("a", 250) + "@b.it", // > 254
	}
	for _, s := range invalid {
		if err := ValidateAddress(s); err == nil {
			t.Errorf("ValidateAddress(%q) = nil, want error", s)
		}
	}
}

func TestSendShareEmailRejectsInvalidRecipient(t *testing.T) {
	err := SendShareEmail("a@b.it\r\nBcc: victim@evil.com", "http://x/d/t", "f.txt", 7, false, "u", nil, "", "")
	if err == nil {
		t.Fatal("SendShareEmail with injected recipient = nil, want error")
	}
}
