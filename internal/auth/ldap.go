package auth

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

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
//
// Returns (isAuthenticated, isAdmin, error)
func Authenticate(username, password string, cfg *config.Config) (bool, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, false, nil
	}

	// ── MOCK / DEV MODE ─────────────────────────────────────────────────────
	if cfg.LDAPHost == "mock" {
		return true, true, nil
	}

	// ── TLS CONFIG ──────────────────────────────────────────────────────────
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.LDAPTLSSkipVerify, //nolint:gosec
		ServerName:         ldapHostname(cfg.LDAPHost),
	}

	// ── CONNECT ─────────────────────────────────────────────────────────────
	log.Printf("ldap: dialing %s", cfg.LDAPHost)
	l, err := ldap.DialURL(cfg.LDAPHost, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return false, false, fmt.Errorf("ldap dial: %w", err)
	}
	defer l.Close()

	l.SetTimeout(5 * time.Second)

	// ── StartTLS (per ldap:// su porta 389) ─────────────────────────────────
	if strings.HasPrefix(cfg.LDAPHost, "ldap://") {
		log.Printf("ldap: attempting StartTLS...")
		if err := l.StartTLS(tlsCfg); err != nil {
			log.Printf("ldap: StartTLS failed/ignored: %v. Reconnecting in plain text...", err)
			l.Close()
			// Riconnettiti in chiaro poichè il socket precedente è rimasto "sporco"
			l, err = ldap.DialURL(cfg.LDAPHost)
			if err != nil {
				return false, false, fmt.Errorf("ldap dial (fallback): %w", err)
			}
			l.SetTimeout(5 * time.Second)
		} else {
			log.Printf("ldap: StartTLS successful")
		}
	}

	// ── USER BIND (verifica credenziali) ────────────────────────────────────
	userDN := fmt.Sprintf(cfg.LDAPUserDNTemplate, username)
	log.Printf("ldap: attempting bind for %s", userDN)
	if err := l.Bind(userDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, false, nil // credenziali errate → login fallito
		}
		return false, false, fmt.Errorf("ldap bind (%s): %w", userDN, err)
	}

	// ── EXPLICIT ADMIN LIST CHECK ───────────────────────────────────────────
	isAdmin := false
	if cfg.AdminUsers != "" {
		for _, adminUser := range strings.Split(cfg.AdminUsers, ";") {
			if strings.EqualFold(strings.TrimSpace(adminUser), username) {
				isAdmin = true
				break
			}
		}
	}

	// ── GROUP MEMBERSHIP CHECK ───────────────────────────────────────────────
	// If no LDAP groups are required (neither for login nor for admin right derivation), we can return early.
	if cfg.LDAPRequiredGroup == "" && cfg.LDAPAdminGroup == "" {
		return true, isAdmin, nil
	}

	// Re-bind con service account se configurato (utile se l'utente non può cercare)
	if cfg.LDAPBindDN != "" {
		if err := l.Bind(cfg.LDAPBindDN, cfg.LDAPBindPassword); err != nil {
			return false, false, fmt.Errorf("ldap service bind: %w", err)
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
		return false, false, fmt.Errorf("ldap search (base=%s, filter=%s): %w", cfg.LDAPBaseDN, searchFilter, err)
	}
	if len(result.Entries) == 0 {
		return false, false, fmt.Errorf("ldap: user %q not found under base DN %q", username, cfg.LDAPBaseDN)
	}

	// Verifica membership per il login e per admin rights
	requiredLower := strings.ToLower(cfg.LDAPRequiredGroup)
	adminLower := strings.ToLower(cfg.LDAPAdminGroup)

	hasRequiredGroup := (cfg.LDAPRequiredGroup == "")

	for _, memberOf := range result.Entries[0].GetAttributeValues("memberOf") {
		memberLower := strings.ToLower(memberOf)

		// Controllo login group
		if !hasRequiredGroup && requiredLower != "" {
			if strings.Contains(memberLower, "cn="+requiredLower) || strings.EqualFold(memberOf, requiredLower) {
				hasRequiredGroup = true
			}
		}

		// Controllo admin group
		if !isAdmin && adminLower != "" {
			if strings.Contains(memberLower, "cn="+adminLower) || strings.EqualFold(memberOf, adminLower) {
				isAdmin = true
			}
		}
	}

	if !hasRequiredGroup {
		return false, false, fmt.Errorf("ldap: user %q authenticated but not in group %q", username, cfg.LDAPRequiredGroup)
	}

	return true, isAdmin, nil
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
