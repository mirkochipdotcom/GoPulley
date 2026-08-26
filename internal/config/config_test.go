package config

import "testing"

func TestGetEnv(t *testing.T) {
	t.Run("returns override when set", func(t *testing.T) {
		t.Setenv("TEST_GETENV_KEY", "override")
		if got := getEnv("TEST_GETENV_KEY", "fallback"); got != "override" {
			t.Fatalf("getEnv() = %q, want %q", got, "override")
		}
	})

	t.Run("returns fallback when unset", func(t *testing.T) {
		if got := getEnv("TEST_GETENV_UNSET", "fallback"); got != "fallback" {
			t.Fatalf("getEnv() = %q, want %q", got, "fallback")
		}
	})

	t.Run("returns fallback when empty string", func(t *testing.T) {
		t.Setenv("TEST_GETENV_EMPTY", "")
		if got := getEnv("TEST_GETENV_EMPTY", "fallback"); got != "fallback" {
			t.Fatalf("getEnv() = %q, want %q", got, "fallback")
		}
	})
}

func TestGetEnvInt(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		set      bool
		fallback int
		want     int
	}{
		{"valid override", "42", true, 10, 42},
		{"unset uses fallback", "", false, 10, 10},
		{"invalid falls back", "not-a-number", true, 10, 10},
		{"negative override", "-5", true, 10, -5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("TEST_GETENVINT_KEY", tc.value)
			}
			if got := getEnvInt("TEST_GETENVINT_KEY", tc.fallback); got != tc.want {
				t.Fatalf("getEnvInt() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		set      bool
		fallback bool
		want     bool
	}{
		{"true literal", "true", true, false, true},
		{"1 literal", "1", true, false, true},
		{"yes literal", "yes", true, false, true},
		{"false literal", "false", true, true, false},
		{"0 literal", "0", true, true, false},
		{"no literal", "no", true, true, false},
		{"unset uses fallback", "", false, true, true},
		{"invalid value uses fallback", "maybe", true, true, true},
		{"case insensitive TRUE", "TRUE", true, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("TEST_GETENVBOOL_KEY", tc.value)
			}
			if got := getEnvBool("TEST_GETENVBOOL_KEY", tc.fallback); got != tc.want {
				t.Fatalf("getEnvBool() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	// No env vars set: Load() must return documented defaults.
	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.LDAPHost != "mock" {
		t.Errorf("LDAPHost = %q, want %q", cfg.LDAPHost, "mock")
	}
	if cfg.MaxGlobalDays != 30 {
		t.Errorf("MaxGlobalDays = %d, want 30", cfg.MaxGlobalDays)
	}
	if cfg.MaxUploadSizeMB != 2048 {
		t.Errorf("MaxUploadSizeMB = %d, want 2048", cfg.MaxUploadSizeMB)
	}
	if !cfg.LDAPStartTLS {
		t.Error("LDAPStartTLS = false, want true (default)")
	}
	if !cfg.SecureCookies {
		t.Error("SecureCookies = false, want true (default)")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LoginSessionTTLHours != 24 {
		t.Errorf("LoginSessionTTLHours = %d, want 24", cfg.LoginSessionTTLHours)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("APP_PORT", "9090")
	t.Setenv("MAX_GLOBAL_DAYS", "90")
	t.Setenv("LDAP_TLS_SKIP_VERIFY", "true")
	t.Setenv("SECURE_COOKIES", "false")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.MaxGlobalDays != 90 {
		t.Errorf("MaxGlobalDays = %d, want 90", cfg.MaxGlobalDays)
	}
	if !cfg.LDAPTLSSkipVerify {
		t.Error("LDAPTLSSkipVerify = false, want true")
	}
	if cfg.SecureCookies {
		t.Error("SecureCookies = true, want false")
	}
	// LogLevel is lower-cased by Load().
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
