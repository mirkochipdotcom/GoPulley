package email

import (
	"crypto/tls"
	"fmt"
	"html"
	"log"
	"net"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/youorg/gopulley/internal/config"
)

// SendShareEmail sends an email with the download link to the specified recipient.
// If optionalUserEmail and optionalUserPassword are provided, they take precedence
// over the global SMTP configuration (useful when SMTPUserAuth is enabled).
func SendShareEmail(to string, downloadURL string, filename string, expirationDays int, isPasswordProtected bool, senderUsername string, cfg *config.Config, optionalUserEmail string, optionalUserPassword string) error {
	smtpServer := cfg.SMTPServer
	smtpPort := cfg.SMTPPort
	smtpSecurity := strings.ToLower(strings.TrimSpace(cfg.SMTPSecurity))
	smtpFrom := cfg.SMTPFrom
	smtpUser := cfg.SMTPUser
	smtpPassword := cfg.SMTPPassword

	// If AD user auth is configured and we received credentials, override globals
	if cfg.SMTPUserAuth && optionalUserEmail != "" && optionalUserPassword != "" {
		smtpUser = optionalUserEmail
		smtpPassword = optionalUserPassword

		// Typically the AD user email is their username or UPN. Use it as FROM unless
		// it lacks an @ domain, in which case we fall back to SMTPFrom or append a guessed domain.
		if strings.Contains(optionalUserEmail, "@") {
			smtpFrom = optionalUserEmail
		}
	}

	if smtpServer == "" || smtpPort == 0 || smtpFrom == "" {
		log.Printf("email: SMTP not fully configured or missing Sender info, skipping email to %s", to)
		return nil
	}
	if smtpSecurity == "" {
		smtpSecurity = "auto"
	}
	if smtpSecurity != "auto" && smtpSecurity != "starttls" && smtpSecurity != "ssl" && smtpSecurity != "none" {
		return fmt.Errorf("unsupported SMTP_SECURITY %q (allowed: auto,starttls,ssl,none)", smtpSecurity)
	}

	var auth smtp.Auth
	if smtpUser != "" || smtpPassword != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPassword, smtpServer)
	}

	// Compose a branded, app-consistent HTML email (headers are built in buildEmailMessage).
	subjectValue := "File condiviso: " + filename
	senderLabel := strings.TrimSpace(senderUsername)
	if senderLabel == "" {
		senderLabel = strings.TrimSpace(optionalUserEmail)
	}
	if senderLabel == "" {
		senderLabel = strings.TrimSpace(smtpFrom)
	}

	body := buildBrandedShareEmail(downloadURL, filename, expirationDays, isPasswordProtected, senderLabel, cfg)

	msg := buildEmailMessage(smtpFrom, to, subjectValue, body)
	addr := fmt.Sprintf("%s:%d", smtpServer, smtpPort)

	log.Printf("email: Sending email to %s for file %s via SMTP_SECURITY=%s", to, filename, smtpSecurity)
	err := sendMailWithMode(addr, smtpServer, smtpSecurity, auth, smtpFrom, []string{to}, msg)
	if err != nil {
		log.Printf("email: Failed to send email to %s: %v", to, err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("email: Successfully sent email to %s", to)
	return nil
}

func buildBrandedShareEmail(downloadURL, filename string, expirationDays int, isPasswordProtected bool, senderLabel string, cfg *config.Config) string {
	brandName := strings.TrimSpace(cfg.BrandName)
	if brandName == "" {
		brandName = "GoPulley"
	}

	baseURL := baseURLFromDownload(downloadURL)
	brandLogoURL := resolveEmailBrandLogoURL(cfg, baseURL)
	appLogoURL := ""
	if baseURL != "" {
		appLogoURL = strings.TrimRight(baseURL, "/") + "/static/img/logo-icon.svg"
	}

	escFile := html.EscapeString(filename)
	escLink := html.EscapeString(downloadURL)
	escBrand := html.EscapeString(brandName)
	escTitle := html.EscapeString("File condiviso: " + filename)
	escSender := html.EscapeString(senderLabel)

	logoBlock := ""
	if brandLogoURL != "" {
		logoBlock = fmt.Sprintf(`<img src="%s" alt="%s" width="40" height="40" style="display:block;width:40px;height:40px;border-radius:10px;border:1px solid rgba(255,255,255,0.18);object-fit:cover;background:#111827;">`, html.EscapeString(brandLogoURL), escBrand)
	} else if appLogoURL != "" {
		logoBlock = fmt.Sprintf(`<img src="%s" alt="GoPulley" width="40" height="40" style="display:block;width:40px;height:40px;border-radius:10px;border:1px solid rgba(255,255,255,0.18);object-fit:contain;background:#111827;padding:6px;">`, html.EscapeString(appLogoURL))
	}

	metaBrand := escBrand
	if escBrand != "GoPulley" {
		metaBrand = escBrand + " - GoPulley"
	}

	passwordNotice := ""
	if isPasswordProtected {
		passwordNotice = `<tr>
					<td style="padding:14px 24px 0 24px;">
						<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:rgba(245,158,11,0.12);border:1px solid rgba(245,158,11,0.35);border-radius:12px;">
							<tr>
								<td style="padding:12px 14px;color:#fde68a;font-size:13px;line-height:1.55;">
									<strong style="color:#fbbf24;">Link protetto da password.</strong> Il destinatario dovra' ricevere una comunicazione separata con la password di accesso.
								</td>
							</tr>
						</table>
					</td>
				</tr>`
	}

	unattendedMailboxNotice := ""
	if !cfg.SMTPUserAuth {
		unattendedMailboxNotice = `<tr>
					<td style="padding:14px 24px 0 24px;">
						<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:rgba(239,68,68,0.10);border:1px solid rgba(239,68,68,0.32);border-radius:12px;">
							<tr>
								<td style="padding:12px 14px;color:#fecaca;font-size:12px;line-height:1.55;">
									Questa email e' stata inviata da una casella non presidiata. Non rispondere a questo messaggio.
								</td>
							</tr>
						</table>
					</td>
				</tr>`
	}

	return fmt.Sprintf(`
<!doctype html>
<html lang="it">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
</head>
<body style="margin:0;padding:0;background:#0d0f14;color:#e8eaf0;font-family:Inter,Segoe UI,Roboto,Arial,sans-serif;">
	<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#0d0f14;padding:28px 12px;">
		<tr>
			<td align="center">
				<table role="presentation" width="620" cellspacing="0" cellpadding="0" style="max-width:620px;width:100%%;background:#15181f;border:1px solid rgba(255,255,255,0.1);border-radius:18px;overflow:hidden;box-shadow:0 16px 40px rgba(0,0,0,0.45);">
					<tr>
						<td style="padding:22px 24px;background:linear-gradient(135deg,#6366f1 0%%,#8b5cf6 100%%);">
							<table role="presentation" width="100%%" cellspacing="0" cellpadding="0">
								<tr>
									<td valign="middle" style="font-size:20px;font-weight:700;line-height:1.2;color:#ffffff;">
										%s
									</td>
									<td align="right" valign="middle" style="width:56px;">
										%s
									</td>
								</tr>
							</table>
						</td>
					</tr>

					<tr>
						<td style="padding:26px 24px 12px 24px;">
							<p style="margin:0 0 10px 0;font-size:20px;line-height:1.35;font-weight:700;color:#e8eaf0;">Nuovo file condiviso</p>
							<p style="margin:0;color:#9ca3b0;font-size:14px;line-height:1.6;">Hai ricevuto un link di download tramite %s.</p>
						</td>
					</tr>

					<tr>
						<td style="padding:0 24px 0 24px;">
							<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#131725;border:1px solid rgba(99,102,241,0.35);border-radius:12px;">
								<tr>
									<td style="padding:10px 14px;color:#c7d2fe;font-size:13px;line-height:1.5;">
										Condivisione generata da: <strong style="color:#ffffff;">%s</strong>
									</td>
								</tr>
							</table>
						</td>
					</tr>

					<tr>
						<td style="padding:12px 24px 0 24px;">
							<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#1c2030;border:1px solid rgba(255,255,255,0.08);border-radius:12px;">
								<tr>
									<td style="padding:14px 16px;">
										<p style="margin:0 0 6px 0;color:#9ca3b0;font-size:12px;text-transform:uppercase;letter-spacing:.04em;">File</p>
										<p style="margin:0;color:#e8eaf0;font-size:15px;font-weight:600;word-break:break-word;">%s</p>
									</td>
								</tr>
							</table>
						</td>
					</tr>

					<tr>
						<td style="padding:18px 24px 0 24px;">
							<table role="presentation" width="100%%" cellspacing="0" cellpadding="0">
								<tr>
									<td align="left" valign="middle">
										<a href="%s" style="display:inline-block;padding:12px 20px;border-radius:10px;background:linear-gradient(135deg,#6366f1 0%%,#8b5cf6 100%%);color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;">Scarica file</a>
									</td>
									<td align="right" valign="middle">
										<span style="display:inline-block;padding:6px 10px;background:rgba(245,158,11,0.15);border:1px solid rgba(245,158,11,0.35);border-radius:999px;color:#fbbf24;font-size:12px;font-weight:600;white-space:nowrap;">Scade tra %d giorni</span>
									</td>
								</tr>
							</table>
						</td>
					</tr>

					%s

					%s

					<tr>
						<td style="padding:18px 24px 8px 24px;">
							<p style="margin:0;color:#9ca3b0;font-size:13px;line-height:1.6;">Se il pulsante non funziona, copia e incolla questo link nel browser:</p>
							<p style="margin:8px 0 0 0;word-break:break-all;"><a href="%s" style="color:#8b5cf6;font-size:13px;text-decoration:underline;">%s</a></p>
						</td>
					</tr>

					<tr>
						<td style="padding:18px 24px 24px 24px;">
							<p style="margin:0;color:#5c6070;font-size:12px;line-height:1.5;">Messaggio automatico inviato da %s. Se non ti aspettavi questa email, puoi ignorarla.</p>
						</td>
					</tr>
				</table>
			</td>
		</tr>
	</table>
</body>
</html>
`, escTitle, escBrand, logoBlock, metaBrand, escSender, escFile, escLink, expirationDays, passwordNotice, unattendedMailboxNotice, escLink, escLink, metaBrand)
}

func baseURLFromDownload(downloadURL string) string {
	u, err := url.Parse(downloadURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func resolveEmailBrandLogoURL(cfg *config.Config, baseURL string) string {
	logo := strings.TrimSpace(cfg.BrandLogoPath)
	if logo == "" {
		return ""
	}
	if strings.HasPrefix(logo, "http://") || strings.HasPrefix(logo, "https://") {
		return logo
	}
	if baseURL == "" {
		return ""
	}
	// Keep parity with app behavior: local brand logos are served by /brand-logo.
	return strings.TrimRight(baseURL, "/") + "/brand-logo"
}

func buildEmailMessage(from string, to string, subject string, htmlBody string) []byte {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost"
	}

	now := time.Now().UTC().Format(time.RFC1123Z)
	msgID := fmt.Sprintf("<%d.%s@gopulley.%s>", time.Now().UTC().UnixNano(), strings.ReplaceAll(hostname, " ", "-"), hostname)

	// RFC-compliant CRLF line endings prevent strict relays from rejecting DATA finalization.
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + now,
		"Message-ID: " + msgID,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	var b strings.Builder
	for _, h := range headers {
		b.WriteString(h)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(htmlBody, "\n", "\r\n"))

	return []byte(b.String())
}

func sendMailWithMode(addr, server, mode string, auth smtp.Auth, from string, to []string, msg []byte) error {
	switch mode {
	case "ssl":
		return sendMailSSL(addr, server, auth, from, to, msg)
	case "starttls":
		return sendMailSMTP(addr, server, true, true, auth, from, to, msg)
	case "none":
		return sendMailSMTP(addr, server, false, false, auth, from, to, msg)
	case "auto":
		return sendMailSMTP(addr, server, false, true, auth, from, to, msg)
	default:
		return fmt.Errorf("unsupported SMTP mode: %s", mode)
	}
}

func sendMailSSL(addr, server string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: server, MinVersion: tls.VersionTLS12}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("smtp ssl dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, server)
	if err != nil {
		return fmt.Errorf("smtp ssl client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := writeMessage(client, from, to, msg); err != nil {
		return err
	}

	return client.Quit()
}

func sendMailSMTP(addr, server string, requireStartTLS bool, autoStartTLS bool, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, server)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	startTLSSupported, _ := client.Extension("STARTTLS")
	if requireStartTLS && !startTLSSupported {
		return fmt.Errorf("smtp server does not support STARTTLS")
	}
	if startTLSSupported && (requireStartTLS || autoStartTLS) {
		tlsConfig := &tls.Config{ServerName: server, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if auth != nil {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("smtp server does not support AUTH")
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := writeMessage(client, from, to, msg); err != nil {
		return err
	}

	return client.Quit()
}

func writeMessage(client *smtp.Client, from string, to []string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt to %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp finalize: %w", err)
	}
	return nil
}
