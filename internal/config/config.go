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
	// EnableSHA256: se true, calcola e memorizza lo SHA-256 di ogni file caricato
	// e lo mostra nella pagina di download.
	EnableSHA256 bool

	// LDAPAdminGroup: AD group defining admins.
	LDAPAdminGroup string
	// AdminUsers: ';' separated list of explicit admin usernames (e.g. name.surname).
	AdminUsers string
	// UserQuotaMB: Max total space a user can fill.
	UserQuotaMB int64
	// UploadChunkSizeMB: default chunk size for chunked uploads (default 10 MB).
	UploadChunkSizeMB int64
	// UploadSessionTTLHours: how long an in-progress upload session is kept before
	// being cleaned up by the background job (default 24 hours).
	UploadSessionTTLHours int
	// MaxUploadSessionsPerUser: concurrent in-progress upload sessions allowed per user.
	MaxUploadSessionsPerUser int
}

// Load reads environment variables and returns a populated Config.
// Falls back to sensible defaults when variables are absent.
func Load() *Config {
	return &Config{
		Port:                     getEnv("APP_PORT", "8080"),
		PublicBaseURL:            getEnv("PUBLIC_BASE_URL", ""),
		SessionSecret:            getEnv("SESSION_SECRET", "please-change-me"),
		LDAPHost:                 getEnv("LDAP_HOST", "mock"),
		LDAPBaseDN:               getEnv("LDAP_BASE_DN", "dc=example,dc=com"),
		LDAPUserDNTemplate:       getEnv("LDAP_USER_DN_TEMPLATE", "uid=%s,ou=Users,dc=example,dc=com"),
		LDAPTLSSkipVerify:        getEnvBool("LDAP_TLS_SKIP_VERIFY", false),
		LDAPBindDN:               getEnv("LDAP_BIND_DN", ""),
		LDAPBindPassword:         getEnv("LDAP_BIND_PASSWORD", ""),
		LDAPRequiredGroup:        getEnv("LDAP_REQUIRED_GROUP", ""),
		MaxGlobalDays:            getEnvInt("MAX_GLOBAL_DAYS", 30),
		MaxUploadSizeMB:          int64(getEnvInt("MAX_UPLOAD_SIZE_MB", 2048)),
		DBPath:                   getEnv("DB_PATH", "./gopulley.db"),
		UploadDir:                getEnv("UPLOAD_DIR", "./uploads"),
		BrandName:                getEnv("BRAND_NAME", ""),
		BrandLogoPath:            getEnv("BRAND_LOGO", ""),
		EnableSHA256:             getEnvBool("ENABLE_SHA256", false),
		LDAPAdminGroup:           getEnv("LDAP_ADMIN_GROUP", ""),
		AdminUsers:               getEnv("ADMIN_USERS", ""),
		UserQuotaMB:              int64(getEnvInt("USER_QUOTA_MB", 0)),
		UploadChunkSizeMB:        int64(getEnvInt("UPLOAD_CHUNK_SIZE_MB", 10)),
		UploadSessionTTLHours:    getEnvInt("UPLOAD_SESSION_TTL_HOURS", 24),
		MaxUploadSessionsPerUser: getEnvInt("MAX_UPLOAD_SESSIONS_PER_USER", 3),
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
