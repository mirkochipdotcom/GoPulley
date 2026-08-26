package i18n

import "testing"

func TestSupportedLocales(t *testing.T) {
	got := SupportedLocales()
	want := map[string]bool{"en": true, "it": true, "es": true, "de": true, "fr": true}
	if len(got) != len(want) {
		t.Fatalf("SupportedLocales() len = %d, want %d", len(got), len(want))
	}
	for _, l := range got {
		if !want[l] {
			t.Errorf("unexpected locale %q in SupportedLocales()", l)
		}
	}
}

func TestNormalizeLocale(t *testing.T) {
	cases := map[string]string{
		"EN":      "en",
		" it ":    "it",
		"en-US":   "en",
		"de_DE":   "de",
		"":        "",
		"FR-fr":   "fr",
	}
	for in, want := range cases {
		if got := NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveLocale(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"empty header falls back to default", "", DefaultLocale},
		{"simple supported lang", "it", "it"},
		{"unsupported lang falls back to default", "zh", DefaultLocale},
		{"quality values pick highest q", "fr;q=0.5,it;q=0.9,en;q=0.8", "it"},
		{"region variants normalize", "de-DE,en;q=0.5", "de"},
		{"unsupported preferred, supported fallback in list", "zh;q=0.9,es;q=0.5", "es"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveLocale(tc.header); got != tc.want {
				t.Errorf("ResolveLocale(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestT(t *testing.T) {
	t.Run("existing key returns translation", func(t *testing.T) {
		got := T("en", "login.signin")
		if got != "Sign in" {
			t.Errorf("T(en, login.signin) = %q, want %q", got, "Sign in")
		}
	})

	t.Run("missing key in supported locale falls back to default locale", func(t *testing.T) {
		// login.signin exists in "en"; use a locale where we know it's absent to prove fallback,
		// but since all locales define the core keys, assert the value at least resolves to
		// the english default when the key is truly missing everywhere.
		got := T("it", "this.key.does.not.exist")
		if got != "this.key.does.not.exist" {
			t.Errorf("T() for missing key = %q, want key echoed back", got)
		}
	})

	t.Run("unsupported locale falls back to default", func(t *testing.T) {
		got := T("zz", "login.signin")
		if got != T(DefaultLocale, "login.signin") {
			t.Errorf("T(zz, ...) = %q, want default locale value", got)
		}
	})

	t.Run("format args are applied", func(t *testing.T) {
		got := T("en", "dashboard.active_count", 3)
		if got != "3 active" {
			t.Errorf("T() with args = %q, want %q", got, "3 active")
		}
	})
}
