package auth

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/youorg/gopulley/internal/config"
)

// Authenticate verifies username/password against the configured LDAP/AD server
// and, if LDAP_REQUIRED_GROUP is set, checks that the user is a member of that group.
//
// If cfg.LDAPHost is "mock", any non-empty credentials are accepted (dev mode).
func Authenticate(username, password string, cfg *config.Config) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, nil
	}

	// ── MOCK / DEV MODE ─────────────────────────────────────────────────────
	if cfg.LDAPHost == "mock" {
		return true, nil
	}

	// ── CONNECT ─────────────────────────────────────────────────────────────
	l, err := ldap.DialURL(cfg.LDAPHost)
	if err != nil {
		return false, fmt.Errorf("ldap dial: %w", err)
	}
	defer l.Close()

	// ── USER BIND (credential check) ─────────────────────────────────────────
	userDN := fmt.Sprintf(cfg.LDAPUserDNTemplate, username)
	if err := l.Bind(userDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, nil
		}
		return false, fmt.Errorf("ldap bind: %w", err)
	}

	// ── GROUP MEMBERSHIP CHECK ───────────────────────────────────────────────
	// If LDAP_REQUIRED_GROUP is set, search for the user and verify membership.
	if cfg.LDAPRequiredGroup == "" {
		return true, nil
	}

	// Optionally re-bind with a service account for the search query.
	// This is needed when the user account doesn't have search privileges.
	if cfg.LDAPBindDN != "" {
		if err := l.Bind(cfg.LDAPBindDN, cfg.LDAPBindPassword); err != nil {
			return false, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	// Search for the user object and retrieve its memberOf attributes.
	// We support both sAMAccountName (AD) and uid (OpenLDAP) login styles.
	searchFilter := fmt.Sprintf(
		"(|(sAMAccountName=%s)(userPrincipalName=%s)(uid=%s))",
		ldap.EscapeFilter(username),
		ldap.EscapeFilter(username),
		ldap.EscapeFilter(username),
	)
	searchReq := ldap.NewSearchRequest(
		cfg.LDAPBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		searchFilter,
		[]string{"memberOf"},
		nil,
	)

	result, err := l.Search(searchReq)
	if err != nil {
		return false, fmt.Errorf("ldap search: %w", err)
	}
	if len(result.Entries) == 0 {
		return false, nil // user not found in directory
	}

	// Check all memberOf values for the required group name (case-insensitive).
	requiredLower := strings.ToLower(cfg.LDAPRequiredGroup)
	for _, memberOf := range result.Entries[0].GetAttributeValues("memberOf") {
		// memberOf values are full DNs like:
		//   CN=ALL_USERS-SERVIZI_INTERNI,OU=Gruppi,DC=intranet,...
		// Match either the full DN or just the CN= part.
		memberLower := strings.ToLower(memberOf)
		if strings.Contains(memberLower, "cn="+requiredLower) ||
			strings.EqualFold(memberOf, requiredLower) {
			return true, nil
		}
	}

	// User authenticated but is not a member of the required group.
	return false, nil
}
