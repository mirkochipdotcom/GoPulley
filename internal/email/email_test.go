package email

import (
	"mime"
	"strings"
	"testing"
)

// headers returns the header block of a message built by buildEmailMessage.
func headers(t *testing.T, msg []byte) []string {
	t.Helper()
	head, _, ok := strings.Cut(string(msg), "\r\n\r\n")
	if !ok {
		t.Fatal("message has no header/body separator")
	}
	return strings.Split(head, "\r\n")
}

func TestBuildEmailMessage_SubjectHeaderInjection(t *testing.T) {
	subject := "File condiviso: x.txt\r\nBcc: victim@example.com\r\n\r\n<h1>injected</h1>"
	msg := buildEmailMessage("from@example.com", "to@example.com", subject, "<p>body</p>")

	for _, h := range headers(t, msg) {
		if strings.HasPrefix(strings.ToLower(h), "bcc:") {
			t.Fatalf("injected header found: %q", h)
		}
	}
	if strings.Contains(string(msg), "\r\n\r\n<h1>injected</h1>") {
		t.Fatal("injected body found")
	}
}

func TestBuildEmailMessage_SubjectEncoding(t *testing.T) {
	subject := "File condiviso: fattura è €.pdf"
	msg := buildEmailMessage("from@example.com", "to@example.com", subject, "<p>body</p>")

	var got string
	for _, h := range headers(t, msg) {
		if v, ok := strings.CutPrefix(h, "Subject: "); ok {
			got = v
		}
	}
	if got == "" {
		t.Fatal("Subject header missing")
	}
	for _, r := range got {
		if r > 0x7e {
			t.Fatalf("Subject header is not ASCII: %q", got)
		}
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(got)
	if err != nil {
		t.Fatalf("decode Subject: %v", err)
	}
	if decoded != subject {
		t.Errorf("decoded Subject = %q, want %q", decoded, subject)
	}
}

func TestBuildEmailMessage_PlainASCIISubjectUnchanged(t *testing.T) {
	msg := buildEmailMessage("from@example.com", "to@example.com", "File condiviso: report.pdf", "<p>body</p>")
	if !strings.Contains(string(msg), "\r\nSubject: File condiviso: report.pdf\r\n") {
		t.Errorf("ASCII Subject should be left as is:\n%s", msg)
	}
}
