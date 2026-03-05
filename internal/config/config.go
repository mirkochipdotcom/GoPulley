package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port string
	// PublicBaseURL è la base URL pubblica usata per generare i link di download
	// (es. https://files.example.com). Obbligatorio in setup a porte separate.
	// Se vuoto, viene inferita dall'Host della request.
	PublicBaseURL      string
	SessionSecret      string
	LDAPHost           string
	LDAPBaseDN         string
	LDAPUserDNTemplate string
	// LDAPTLSSkipVerify: impostare a true solo per DC con certificato self-signed.
	// Mai usare in produzione con certificati validi.
	LDAPTLSSkipVerify bool
	// LDAPBindDN + LDAPBindPassword: service account used for group-membership searches.
	// If empty, the user's own authenticated session is reused.
	LDAPBindDN       string
	LDAPBindPassword string
	// LDAPRequiredGroup: CN of the AD group required to log in (empty = allow all).
	LDAPRequiredGroup string
	MaxGlobalDays     int
	MaxUploadSizeMB   int64
	DBPath            string
	UploadDir         string
	// Branding aziendale opzionale
	// BrandName: nome dell'ente/azienda da affiancare al logo GoPulley
	// BrandLogoPath: path relativo a /static/ del logo PNG (es. "img/brand-logo.png")
	BrandName     string
	BrandLogoPath string
}

// Load reads environment variables and returns a populated Config.
// Falls back to sensible defaults when variables are absent.
func Load() *Config {
	return &Config{
		Port:               getEnv("APP_PORT", "8080"),
		PublicBaseURL:      getEnv("PUBLIC_BASE_URL", ""),
		SessionSecret:      getEnv("SESSION_SECRET", "please-change-me"),
		LDAPHost:           getEnv("LDAP_HOST", "mock"),
		LDAPBaseDN:         getEnv("LDAP_BASE_DN", "dc=example,dc=com"),
		LDAPUserDNTemplate: getEnv("LDAP_USER_DN_TEMPLATE", "uid=%s,ou=Users,dc=example,dc=com"),
		LDAPTLSSkipVerify:  getEnvBool("LDAP_TLS_SKIP_VERIFY", false),
		LDAPBindDN:         getEnv("LDAP_BIND_DN", ""),
		LDAPBindPassword:   getEnv("LDAP_BIND_PASSWORD", ""),
		LDAPRequiredGroup:  getEnv("LDAP_REQUIRED_GROUP", ""),
		MaxGlobalDays:      getEnvInt("MAX_GLOBAL_DAYS", 30),
		MaxUploadSizeMB:    int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 2048)),
		DBPath:             getEnv("DB_PATH", "./gopulley.db"),
		UploadDir:          getEnv("UPLOAD_DIR", "./uploads"),
		BrandName:          getEnv("BRAND_NAME", ""),
		BrandLogoPath:      getEnv("BRAND_LOGO", ""),
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

func getEnvBool(key string, fallback bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "true" || v == "1" || v == "yes" {
		return true
	}
	if v == "false" || v == "0" || v == "no" {
		return false
	}
	return fallback
}
