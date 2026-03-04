package auth

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/youorg/gopulley/internal/config"
)

// Authenticate verifies username/password against the configured LDAP/AD server
// and, if LDAP_REQUIRED_GROUP is set, checks that the user is a member of that group.
//
// Supported connection modes (auto-detected from LDAP_HOST):
//   - ldap://host:389  → tries plain, then StartTLS if server requires it
//   - ldaps://host:636 → TLS from the start
//
// Set LDAP_TLS_SKIP_VERIFY=true for self-signed / internal CA certificates.
// Set LDAP_HOST=mock to bypass LDAP entirely (dev mode).
func Authenticate(username, password string, cfg *config.Config) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, nil
	}

	// ── MOCK / DEV MODE ─────────────────────────────────────────────────────
	if cfg.LDAPHost == "mock" {
		return true, nil
	}

	// ── TLS CONFIG ──────────────────────────────────────────────────────────
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.LDAPTLSSkipVerify, //nolint:gosec
		ServerName:         ldapHostname(cfg.LDAPHost),
	}

	// ── CONNECT ─────────────────────────────────────────────────────────────
	l, err := ldap.DialURL(cfg.LDAPHost, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return false, fmt.Errorf("ldap dial: %w", err)
	}
	defer l.Close()

	// ── StartTLS (per ldap:// su porta 389) ─────────────────────────────────
	// Se il server chiude la connessione con EOF in plaintext, proviamo StartTLS.
	if strings.HasPrefix(cfg.LDAPHost, "ldap://") {
		if err := l.StartTLS(tlsCfg); err != nil {
			// StartTLS non supportato/richiesto: continuiamo senza (alcuni AD lo permettono)
			// Il vero errore verrà fuori al momento del bind se necessario.
			_ = err
		}
	}

	// ── USER BIND (verifica credenziali) ────────────────────────────────────
	userDN := fmt.Sprintf(cfg.LDAPUserDNTemplate, username)
	if err := l.Bind(userDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, nil // credenziali errate → login fallito
		}
		return false, fmt.Errorf("ldap bind (%s): %w", userDN, err)
	}

	// ── GROUP MEMBERSHIP CHECK ───────────────────────────────────────────────
	if cfg.LDAPRequiredGroup == "" {
		return true, nil
	}

	// Re-bind con service account se configurato (utile se l'utente non può cercare)
	if cfg.LDAPBindDN != "" {
		if err := l.Bind(cfg.LDAPBindDN, cfg.LDAPBindPassword); err != nil {
			return false, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	// Cerca l'utente e legge memberOf
	searchFilter := fmt.Sprintf(
		"(|(sAMAccountName=%s)(userPrincipalName=%s)(uid=%s))",
		ldap.EscapeFilter(username),
		ldap.EscapeFilter(username),
		ldap.EscapeFilter(strings.Split(username, "@")[0]), // supporta UPN e username puro
	)
	searchReq := ldap.NewSearchRequest(
		cfg.LDAPBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false,
		searchFilter,
		[]string{"memberOf", "sAMAccountName", "cn"},
		nil,
	)

	result, err := l.Search(searchReq)
	if err != nil {
		return false, fmt.Errorf("ldap search (base=%s, filter=%s): %w", cfg.LDAPBaseDN, searchFilter, err)
	}
	if len(result.Entries) == 0 {
		return false, fmt.Errorf("ldap: user %q not found under base DN %q", username, cfg.LDAPBaseDN)
	}

	// Verifica membership (case-insensitive, match CN= nel DN completo)
	requiredLower := strings.ToLower(cfg.LDAPRequiredGroup)
	for _, memberOf := range result.Entries[0].GetAttributeValues("memberOf") {
		memberLower := strings.ToLower(memberOf)
		if strings.Contains(memberLower, "cn="+requiredLower) ||
			strings.EqualFold(memberOf, requiredLower) {
			return true, nil
		}
	}

	// Utente autenticato ma non nel gruppo richiesto
	return false, fmt.Errorf("ldap: user %q authenticated but not in group %q", username, cfg.LDAPRequiredGroup)
}

// ldapHostname estrae l'hostname dal URL LDAP per la verifica TLS.
func ldapHostname(ldapURL string) string {
	s := strings.TrimPrefix(ldapURL, "ldaps://")
	s = strings.TrimPrefix(s, "ldap://")
	if i := strings.Index(s, ":"); i != -1 {
		return s[:i]
	}
	return s
}
