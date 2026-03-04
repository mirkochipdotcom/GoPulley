package config

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port               string
	SessionSecret      string
	LDAPHost           string
	LDAPBaseDN         string
	LDAPUserDNTemplate string
	// LDAPBindDN + LDAPBindPassword: service account used for group-membership searches.
	// If empty, the user's own authenticated session is reused.
	LDAPBindDN         string
	LDAPBindPassword   string
	// LDAPRequiredGroup: CN of the AD group required to log in (empty = allow all).
	LDAPRequiredGroup  string
	MaxGlobalDays      int
	MaxUploadSizeMB    int64
	DBPath             string
	UploadDir          string
}

// Load reads environment variables and returns a populated Config.
// Falls back to sensible defaults when variables are absent.
func Load() *Config {
	return &Config{
		Port:               getEnv("APP_PORT", "8080"),
		SessionSecret:      getEnv("SESSION_SECRET", "please-change-me"),
		LDAPHost:           getEnv("LDAP_HOST", "mock"),
		LDAPBaseDN:         getEnv("LDAP_BASE_DN", "dc=example,dc=com"),
		LDAPUserDNTemplate: getEnv("LDAP_USER_DN_TEMPLATE", "uid=%s,ou=Users,dc=example,dc=com"),
		LDAPBindDN:         getEnv("LDAP_BIND_DN", ""),
		LDAPBindPassword:   getEnv("LDAP_BIND_PASSWORD", ""),
		LDAPRequiredGroup:  getEnv("LDAP_REQUIRED_GROUP", ""),
		MaxGlobalDays:      getEnvInt("MAX_GLOBAL_DAYS", 30),
		MaxUploadSizeMB:    int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 2048)),
		DBPath:             getEnv("DB_PATH", "./gopulley.db"),
		UploadDir:          getEnv("UPLOAD_DIR", "./uploads"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
