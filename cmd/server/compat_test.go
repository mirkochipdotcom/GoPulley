package main

// Test di compatibilità per le dipendenze in auto-merge dependabot
// (gorilla/sessions, golang.org/x/crypto). I valori "golden" sono stati
// generati con le versioni in uso al momento della scrittura: un bump che
// cambia il formato dei cookie o degli hash bcrypt farebbe fallire questi
// test invece di sloggare gli utenti o rompere le share protette in produzione.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const (
	testSessionSecret = "gopulley-test-secret"

	// Cookie di sessione emesso da gorilla/sessions v1.4.0 con
	// newCookieStore(testSessionSecret), username=mario.rossi, is_admin=true.
	goldenSessionCookie = "MTc5MjYxNzE1OXxiWkxWYXduUFRXMTBLXzRTTHYzdWdJMExpM3M1MkdXMXFwMExVaF9nYUVXemRIZ2JqNU16dDNFMHd6a29aYWNwdExGeDZ4VTkzeFdDQ1JmT1lZeUFwcExTVkNBUWEwRmw5U3Q1TGJ2cF8yTkM1TzN2LUJyZTFVVnd3cFd5LUpjNjNXeDdpSTh4WHBBPXxBQ-qMG7kzGgb_c7CAcqmCOuOIk44bFkFf2VlT-EVVRg=="

	// Hash bcrypt di "s3cret-share" generato da golang.org/x/crypto v0.56.0,
	// come quelli salvati in shares.password_hash.
	goldenSharePassword = "s3cret-share"
	goldenShareHash     = "$2a$10$URdSHRqbV/nRpdfMNpBO5ekl.CGKGXYegULsjhUNQA4l/1Sd4lY66"
)

func requestWithCookie(value string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionName, Value: value})
	return req
}

func TestCookieStore_RoundTrip(t *testing.T) {
	store := newCookieStore(testSessionSecret)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	sess, err := store.New(req, sessionName)
	if err != nil {
		t.Fatalf("store.New() error: %v", err)
	}
	sess.Values["username"] = "mario.rossi"
	sess.Values["is_admin"] = true
	if err := sess.Save(req, rec); err != nil {
		t.Fatalf("sess.Save() error: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}

	got, err := newCookieStore(testSessionSecret).Get(requestWithCookie(cookies[0].Value), sessionName)
	if err != nil {
		t.Fatalf("store.Get() error: %v", err)
	}
	if got.IsNew {
		t.Fatal("session decoded as new, want existing")
	}
	if got.Values["username"] != "mario.rossi" || got.Values["is_admin"] != true {
		t.Errorf("session values = %v", got.Values)
	}
}

func TestCookieStore_DecodesLegacyCookie(t *testing.T) {
	store := newCookieStore(testSessionSecret)
	// Il cookie golden ha un timestamp fisso: disattiva il controllo di scadenza.
	store.MaxAge(0)

	got, err := store.Get(requestWithCookie(goldenSessionCookie), sessionName)
	if err != nil {
		t.Fatalf("store.Get() on legacy cookie error: %v", err)
	}
	if got.IsNew {
		t.Fatal("legacy cookie decoded as new session: existing users would be logged out")
	}
	if got.Values["username"] != "mario.rossi" || got.Values["is_admin"] != true {
		t.Errorf("legacy session values = %v", got.Values)
	}
}

func TestCookieStore_RejectsWrongSecret(t *testing.T) {
	store := newCookieStore("another-secret")
	store.MaxAge(0)

	got, err := store.Get(requestWithCookie(goldenSessionCookie), sessionName)
	if err == nil {
		t.Fatal("store.Get() with wrong secret: want error, got nil")
	}
	if !got.IsNew || len(got.Values) != 0 {
		t.Errorf("wrong secret must yield an empty new session, got %v", got.Values)
	}
}

func TestBcrypt_VerifiesLegacyHash(t *testing.T) {
	if err := bcrypt.CompareHashAndPassword([]byte(goldenShareHash), []byte(goldenSharePassword)); err != nil {
		t.Fatalf("legacy share hash no longer verifies: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(goldenShareHash), []byte("wrong")); err == nil {
		t.Fatal("legacy share hash accepted a wrong password")
	}
}

func TestBcrypt_RoundTrip(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte(goldenSharePassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(goldenSharePassword)); err != nil {
		t.Fatalf("CompareHashAndPassword() on fresh hash: %v", err)
	}
}
