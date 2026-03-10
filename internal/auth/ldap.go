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
	if cfg.LDAPStartTLS && strings.HasPrefix(cfg.LDAPHost, "ldap://") {
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

	// Cerca l'utente per ottenere il suo full DN
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
		[]string{"dn", "memberOf", "sAMAccountName", "cn"},
		nil,
	)

	result, err := l.Search(searchReq)
	if err != nil {
		return false, false, fmt.Errorf("ldap search (base=%s, filter=%s): %w", cfg.LDAPBaseDN, searchFilter, err)
	}
	if len(result.Entries) == 0 {
		return false, false, fmt.Errorf("ldap: user %q not found under base DN %q", username, cfg.LDAPBaseDN)
	}

	userEntry := result.Entries[0]
	userFullDN := userEntry.DN

	// Verifica membership per il login e per admin rights
	requiredGroup := cfg.LDAPRequiredGroup
	adminGroup := cfg.LDAPAdminGroup

	hasRequiredGroup := (requiredGroup == "")

	// 1. Controllo base sugli attributi memberOf (più veloce, per membership diretta o server non-AD)
	requiredLower := strings.ToLower(requiredGroup)
	adminLower := strings.ToLower(adminGroup)

	for _, memberOf := range userEntry.GetAttributeValues("memberOf") {
		memberLower := strings.ToLower(memberOf)

		// Controllo login group
		if !hasRequiredGroup && requiredLower != "" {
			if strings.Contains(memberLower, "cn="+requiredLower+",") || strings.EqualFold(memberOf, requiredLower) || strings.HasPrefix(memberLower, "cn="+requiredLower) {
				hasRequiredGroup = true
			}
		}

		// Controllo admin group
		if !isAdmin && adminLower != "" {
			if strings.Contains(memberLower, "cn="+adminLower+",") || strings.EqualFold(memberOf, adminLower) || strings.HasPrefix(memberLower, "cn="+adminLower) {
				isAdmin = true
			}
		}
	}

	// 2. Controllo query nested (LDAP_MATCHING_RULE_IN_CHAIN) se non risolto con memberOf e siamo su AD (non mock)
	if (!hasRequiredGroup && requiredGroup != "") || (!isAdmin && adminGroup != "") {
		log.Printf("ldap: %s: checking nested group membership for required/admin groups (IN_CHAIN mode)", username)
	}

	// Helper per interrogazione IN_CHAIN
	checkNestedMembership := func(groupName string, userDN string) bool {
		if groupName == "" {
			return false
		}

		// Gestione: groupName può essere solo il CN (es "gruppoA") o un full DN.
		// Spesso in cfg le persone mettono solo "gruppoA". Cerchiamo un oggetto group con quel CN e member IN_CHAIN userDN.
		var nestedFilter string
		if strings.Contains(groupName, "=") {
			// Probabilmente un DN completo, es "CN=GruppoA,OU=Groups,DC=example,DC=com"
			nestedFilter = fmt.Sprintf(
				"(&(objectClass=group)(distinguishedName=%s)(member:1.2.840.113556.1.4.1941:=%s))",
				ldap.EscapeFilter(groupName),
				ldap.EscapeFilter(userDN),
			)
		} else {
			// Solo il nome del gruppo, es "gruppoA"
			nestedFilter = fmt.Sprintf(
				"(&(objectClass=group)(cn=%s)(member:1.2.840.113556.1.4.1941:=%s))",
				ldap.EscapeFilter(groupName),
				ldap.EscapeFilter(userDN),
			)
		}

		nestedReq := ldap.NewSearchRequest(
			cfg.LDAPBaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false,
			nestedFilter,
			[]string{"cn"},
			nil,
		)

		nestedRes, errNested := l.Search(nestedReq)
		if errNested != nil {
			log.Printf("ldap: error checking nested membership for %q in %q: %v", userDN, groupName, errNested)
			return false
		}
		return len(nestedRes.Entries) > 0
	}

	if !hasRequiredGroup && requiredGroup != "" {
		if checkNestedMembership(requiredGroup, userFullDN) {
			hasRequiredGroup = true
		}
	}

	if !isAdmin && adminGroup != "" {
		if checkNestedMembership(adminGroup, userFullDN) {
			isAdmin = true
		}
	}

	if !hasRequiredGroup {
		return false, false, fmt.Errorf("ldap: user %q authenticated but not in required group %q (direct or nested)", username, cfg.LDAPRequiredGroup)
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
