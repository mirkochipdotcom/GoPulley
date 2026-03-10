package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/youorg/gopulley/internal/auth"
	"github.com/youorg/gopulley/internal/config"
	"github.com/youorg/gopulley/internal/database"
	"github.com/youorg/gopulley/internal/email"
	"github.com/youorg/gopulley/internal/i18n"
	"github.com/youorg/gopulley/internal/logger"
	"github.com/youorg/gopulley/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// brandLogoSrc risolve il logo aziendale in un URL pronto per src=.
// Se il valore inizia con http/https viene usato direttamente (URL esterno);
// altrimenti viene trattato come path relativo alla cartella /static/.
func brandLogoSrc(logo string) string {
	if strings.HasPrefix(logo, "http://") || strings.HasPrefix(logo, "https://") {
		return logo
	}
	return "/brand-logo"
}

func (a *App) handleBrandLogo(w http.ResponseWriter, r *http.Request) {
	if a.cfg.BrandLogoPath == "" {
		http.Error(w, "not found", 404)
		return
	}
	// Logo is stored in /data (e.g. /data/my-logo.png)
	logoPath := filepath.Join(filepath.Dir(a.cfg.DBPath), a.cfg.BrandLogoPath)
	http.ServeFile(w, r, logoPath)
}

// shouldUseSecureCookie determina se il flag Secure del cookie deve essere true.
// Priorità: 1) X-Forwarded-Proto header (da reverse proxy), 2) configurazione statica.
func shouldUseSecureCookie(r *http.Request, cfg *config.Config) bool {
	// Se il reverse proxy ha impostato X-Forwarded-Proto, fidati di quello
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto == "https"
	}
	// Altrimenti fallback sulla configurazione statica
	return cfg.SecureCookies
}

// ── App ─────────────────────────────────────────────────────────────────────

// AppVersion is injected at build time via -ldflags "-X main.AppVersion=<ver>"
var AppVersion = "unknown"

type App struct {
	cfg       *config.Config
	db        *database.DB
	store     *sessions.CookieStore
	templates map[string]*template.Template
}

const sessionName = "gopulley-session"

// ── Template helpers ─────────────────────────────────────────────────────────

func fileIcon(filename string) template.HTML {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "svg", "bmp", "ico":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><path d="M21 15l-5-5L5 21"></path></svg>`)
	case "mp4", "avi", "mkv", "mov", "wmv", "flv", "webm":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="4" width="20" height="16" rx="2"></rect><path d="M10 9l5 3-5 3V9z"></path></svg>`)
	case "mp3", "wav", "flac", "ogg", "aac", "m4a":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 18V6l11-2v12"></path><circle cx="6" cy="18" r="3"></circle><circle cx="17" cy="16" r="3"></circle></svg>`)
	case "zip", "rar", "7z", "tar", "gz", "bz2":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2"></rect><path d="M10 7h4M10 11h4M10 15h4"></path></svg>`)
	case "pdf":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M7 14h2m2 0h2m2 0h2"></path></svg>`)
	case "doc", "docx", "odt", "rtf":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M8 13h8M8 17h8"></path></svg>`)
	case "xls", "xlsx", "ods", "csv":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M8 17V13m4 4V11m4 6V9"></path></svg>`)
	case "ppt", "pptx", "odp":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M8 16l2.5-3 2 2 3.5-4"></path></svg>`)
	case "txt", "md", "log":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><path d="M8 13h8M8 17h8"></path></svg>`)
	case "js", "ts", "go", "py", "java", "c", "cpp", "cs", "rb", "php", "rs", "sh", "sql", "html", "css", "json", "xml", "yaml", "yml":
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M16 18l6-6-6-6"></path><path d="M8 6l-6 6 6 6"></path></svg>`)
	default:
		return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>`)
	}
}

func (a *App) loadTemplates(baseDir string) error {
	funcs := template.FuncMap{
		"seq":          func(vals ...int) []int { return vals },
		"humanSize":    storage.HumanSize,
		"brandLogoSrc": brandLogoSrc,
		"fileIcon":     fileIcon,
		"t":            func(key string, args ...any) string { return i18n.T(i18n.DefaultLocale, key, args...) },
		"fmtDate": func(t time.Time) string {
			return t.Format("02 Jan 2006 15:04")
		},
		"daysLeft": func(t time.Time) string {
			return i18n.DaysLeft(i18n.DefaultLocale, t)
		},
		"isExpiringSoon": func(t time.Time) bool {
			d := time.Until(t)
			return d > 0 && d < 24*time.Hour
		},
		"isExpired": func(t time.Time) bool {
			return time.Now().After(t)
		},
		"daysAgo": func(t time.Time) string {
			return i18n.DaysAgo(i18n.DefaultLocale, t)
		},
		"safeHTMLAttr": func(s string) template.HTMLAttr {
			return template.HTMLAttr(s)
		},
	}

	names := []string{"login", "dashboard", "download", "admin"}
	a.templates = make(map[string]*template.Template, len(names))
	for _, name := range names {
		path := filepath.Join(baseDir, name+".html")
		tmpl, err := template.New(name + ".html").Funcs(funcs).ParseFiles(path)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", name, err)
		}
		a.templates[name] = tmpl
	}
	return nil
}

func (a *App) requestLocale(r *http.Request) string {
	if r == nil {
		return i18n.DefaultLocale
	}
	return i18n.ResolveLocale(r.Header.Get("Accept-Language"))
}

func (a *App) localizedTemplateFuncs(locale string) template.FuncMap {
	return template.FuncMap{
		"t":            func(key string, args ...any) string { return i18n.T(locale, key, args...) },
		"fmtDate":      func(t time.Time) string { return t.Format("02 Jan 2006 15:04") },
		"daysLeft":     func(t time.Time) string { return i18n.DaysLeft(locale, t) },
		"daysAgo":      func(t time.Time) string { return i18n.DaysAgo(locale, t) },
		"safeHTMLAttr": func(s string) template.HTMLAttr { return template.HTMLAttr(s) },
	}
}

func (a *App) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	tmpl, ok := a.templates[name]
	if !ok {
		http.Error(w, "template not found", 500)
		return
	}
	locale := a.requestLocale(r)
	localized, err := tmpl.Clone()
	if err != nil {
		http.Error(w, "template clone error", 500)
		return
	}
	localized = localized.Funcs(a.localizedTemplateFuncs(locale))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := localized.ExecuteTemplate(w, name+".html", data); err != nil {
		logger.Error("render %s: %v", name, err)
	}
}

// ── Session helpers ───────────────────────────────────────────────────────────

func (a *App) getUsername(r *http.Request) string {
	sess, _ := a.store.Get(r, sessionName)
	u, _ := sess.Values["username"].(string)
	return u
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.getUsername(r) == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, _ := a.store.Get(r, sessionName)
		isAdmin, _ := sess.Values["is_admin"].(bool)
		if !isAdmin {
			http.Error(w, "forbidden: admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// publicBaseURL restituisce la base URL pubblica (schema+host) da usare nei link
// di download condivisi. Usa PUBLIC_BASE_URL se configurato, altrimenti lo inferisce
// dalla request (utile solo quando le due porte coincidono o in sviluppo locale).
func (a *App) publicBaseURL(r *http.Request) string {
	if a.cfg.PublicBaseURL != "" {
		return a.cfg.PublicBaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// GET /
func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	if a.getUsername(r) != "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// GET /login
func (a *App) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, "login", map[string]any{
		"MockMode":  a.cfg.LDAPHost == "mock",
		"Version":   AppVersion,
		"BrandName": a.cfg.BrandName,
		"BrandLogo": a.cfg.BrandLogoPath,
	})
}

// POST /login
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	logger.Debug("📥 Received login request for: %s", username)

	ok, isAdmin, err := auth.Authenticate(username, password, a.cfg)
	if err != nil {
		w.Header().Set("HX-Reswap", "outerHTML")
		// Se l'errore indica che le policy sono fallite o l'utente non c'è, è un auth failure, non un errore di rete
		if strings.Contains(err.Error(), "not in required group") {
			logger.Warn("login failed for %s: unauthorized (not in required group '%s')", username, a.cfg.LDAPRequiredGroup)
			a.renderError(w, r, "login.err_invalid")
			return
		} else if strings.Contains(err.Error(), "not found under base DN") {
			logger.Warn("login failed for %s: user not found in directory", username)
			a.renderError(w, r, "login.err_invalid")
			return
		}

		logger.Error("ldap connection/bind error for %s: %v", username, err)
		a.renderError(w, r, "login.err_ldap")
		return
	}
	if !ok {
		logger.Warn("login failed for %s: invalid credentials", username)
		w.Header().Set("HX-Reswap", "outerHTML")
		// HTMX drops 4xx/5xx by default. We return 200 so the error message is displayed.
		a.renderError(w, r, "login.err_invalid")
		return
	}

	logger.Info("login successful for: %s (isAdmin: %t)", username, isAdmin)

	sess, _ := a.store.Get(r, sessionName)
	sess.Values["username"] = username
	sess.Values["is_admin"] = isAdmin

	// If AD user auth is enabled for SMTP, we need their raw password for SMTP later.
	// As the session secret encrypts the cookie data automatically by gorilla/sessions
	// we store it here.
	if a.cfg.SMTPUserAuth {
		sess.Values["smtp_password"] = password
	}

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   shouldUseSecureCookie(r, a.cfg), // Auto-detect da X-Forwarded-Proto o config
	}
	if err := sess.Save(r, w); err != nil {
		logger.Error("session save: %v", err)
	}

	// HTMX redirect
	w.Header().Set("HX-Redirect", "/dashboard")
	w.WriteHeader(http.StatusOK)
}

func (a *App) renderError(w http.ResponseWriter, r *http.Request, msgKey string, args ...any) {
	msg := i18n.T(a.requestLocale(r), msgKey, args...)
	fmt.Fprintf(w, `<p id="login-error" class="error-msg">%s</p>`, template.HTMLEscapeString(msg))
}

// POST /logout
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess, _ := a.store.Get(r, sessionName)
	sess.Options.MaxAge = -1
	sess.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// GET /dashboard
type dashData struct {
	Username     string
	IsAdmin      bool
	Shares       []*database.Share
	EmailEnabled bool
	EnableSHA    bool
	MaxDays      int
	BaseURL      string
	Version      string
	BrandName    string
	BrandLogo    string
	QuotaMB      int64
	UsedMB       int64
	QuotaPercent int
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)
	sess, _ := a.store.Get(r, sessionName)
	isAdmin, _ := sess.Values["is_admin"].(bool)

	shares, err := a.db.ListSharesByUser(username)
	if err != nil {
		logger.Error("list shares: %v", err)
		shares = nil
	}

	var usedMB int64
	var quotaPercent int
	if a.cfg.UserQuotaMB > 0 {
		totalBytes, err := a.db.GetUserTotalBytes(username)
		if err == nil {
			usedMB = totalBytes / (1024 * 1024)
			quotaPercent = int((float64(usedMB) / float64(a.cfg.UserQuotaMB)) * 100)
			if quotaPercent > 100 {
				quotaPercent = 100
			}
		} else {
			logger.Error("get quota bytes error: %v", err)
		}
	}

	baseURL := a.publicBaseURL(r)

	a.render(w, r, "dashboard", dashData{
		Username:     username,
		IsAdmin:      isAdmin,
		Shares:       shares,
		EmailEnabled: strings.TrimSpace(a.cfg.SMTPServer) != "",
		EnableSHA:    a.cfg.EnableSHA256,
		MaxDays:      a.cfg.MaxGlobalDays,
		BaseURL:      baseURL,
		Version:      AppVersion,
		BrandName:    a.cfg.BrandName,
		BrandLogo:    a.cfg.BrandLogoPath,
		QuotaMB:      a.cfg.UserQuotaMB,
		UsedMB:       usedMB,
		QuotaPercent: quotaPercent,
	})
}

// GET /admin
type adminDashData struct {
	Username     string
	Version      string
	BrandName    string
	BrandLogo    string
	Files        []*database.Share
	TotalSpace   int64
	FreeSpace    int64
	UsedSpace    int64
	SpacePercent int
}

func (a *App) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)

	// Recupera tutti i file usando ListAllShares (che va prima aggiunta in sqlite.go)
	// Nota: qui chiameremo un nuovo metodo db.ListAllShares
	files, err := a.db.ListAllShares()
	if err != nil {
		logger.Error("list all shares error: %v", err)
		files = nil
	}

	// Calculate /data disk space
	dataPath := filepath.Dir(a.cfg.DBPath)
	freeSpace, totalSpace, err := storage.GetDiskUsage(dataPath)
	var usedSpace int64
	var spacePercent int
	if err == nil && totalSpace > 0 {
		usedSpace = totalSpace - freeSpace
		spacePercent = int((float64(usedSpace) / float64(totalSpace)) * 100)
		if spacePercent > 100 {
			spacePercent = 100
		}
	} else if err != nil {
		logger.Error("disk usage error for %s: %v", dataPath, err)
	}

	a.render(w, r, "admin", adminDashData{
		Username:     username,
		Version:      AppVersion,
		BrandName:    a.cfg.BrandName,
		BrandLogo:    a.cfg.BrandLogoPath,
		Files:        files,
		TotalSpace:   totalSpace,
		FreeSpace:    freeSpace,
		UsedSpace:    usedSpace,
		SpacePercent: spacePercent,
	})
}

// POST /upload
func (a *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)
	maxBytes := a.cfg.MaxUploadSizeMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	// Days validation
	days := 0
	fmt.Sscanf(r.FormValue("days"), "%d", &days)
	if days < 1 || days > a.cfg.MaxGlobalDays {
		http.Error(w, fmt.Sprintf("invalid duration (1-%d days)", a.cfg.MaxGlobalDays), http.StatusBadRequest)
		return
	}

	maxDownloads := 0
	fmt.Sscanf(r.FormValue("max_downloads"), "%d", &maxDownloads)
	if maxDownloads < 0 {
		maxDownloads = 0
	}

	password := r.FormValue("password")
	var passwordHash string
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("bcrypt error: %v", err)
			http.Error(w, "server error (password)", 500)
			return
		}
		passwordHash = string(hash)
	}

	shareEmail := strings.TrimSpace(r.FormValue("share_email"))

	var userEmail string
	var userPassword string
	if a.cfg.SMTPUserAuth && shareEmail != "" {
		sess, _ := a.store.Get(r, sessionName)
		userEmail = username
		if pwd, ok := sess.Values["smtp_password"].(string); ok {
			userPassword = pwd
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file uploaded", http.StatusBadRequest)
		return
	}
	file.Close()

	// Quota check
	if a.cfg.UserQuotaMB > 0 {
		usedBytes, err := a.db.GetUserTotalBytes(username)
		if err != nil {
			logger.Error("quota check error: %v", err)
			http.Error(w, "server error (quota)", 500)
			return
		}
		if usedBytes+header.Size > a.cfg.UserQuotaMB*1024*1024 {
			http.Error(w, "storage quota exceeded", http.StatusForbidden)
			return
		}
	}

	// Ensure upload dir exists
	if err := os.MkdirAll(a.cfg.UploadDir, 0750); err != nil {
		logger.Error("mkdir uploads: %v", err)
		http.Error(w, "server error", 500)
		return
	}

	filePath, originalName, sizeBytes, err := storage.SaveFile(header, a.cfg.UploadDir)
	if err != nil {
		logger.Error("save file: %v", err)
		http.Error(w, "file save error", 500)
		return
	}

	token := uuid.New().String()
	expiresAt := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)

	share, err := a.db.CreateShare(token, filePath, originalName, sizeBytes, username, expiresAt, "", passwordHash, maxDownloads)
	if err != nil {
		logger.Error("create share: %v", err)
		storage.DeleteFile(filePath)
		http.Error(w, "database error", 500)
		return
	}

	logger.Info("User %s uploaded file %s (size: %s, token: %s)", username, originalName, storage.HumanSize(sizeBytes), token)

	if a.cfg.EnableSHA256 {
		// Compute hash asynchronously to avoid blocking the upload response on large files.
		go func(token, path string) {
			hash, err := storage.ComputeSHA256(path)
			if err != nil {
				logger.Error("compute sha256 async (%s): %v", token, err)
				return
			}
			if err := a.db.SetShareSHA256(token, hash); err != nil {
				logger.Error("store sha256 async (%s): %v", token, err)
			}
		}(share.Token, filePath)
	}

	downloadURL := fmt.Sprintf("%s/d/%s", a.publicBaseURL(r), share.Token)

	if shareEmail != "" {
		go func() {
			err := email.SendShareEmail(shareEmail, downloadURL, originalName, days, passwordHash != "", username, a.cfg, userEmail, userPassword)
			if err != nil {
				logger.Error("failed to send share email to %s: %v", shareEmail, err)
			} else {
				logger.Debug("share email successfully sent to %s", shareEmail)
			}
		}()
	}

	// Return HTMX response: a success card
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
<div id="upload-result" class="upload-result success" role="alert">
  <div class="result-inner">
    <span class="result-icon">✓</span>
    <div class="result-content">
      <strong>%s</strong> uploaded successfully!<br>
      <small>%s &middot; expires in %d days</small>
    </div>
  </div>
  <div class="link-box">
    <input type="text" value="%s" readonly id="share-link" />
    <button class="btn btn-copy" onclick="copyLink()">Copy link</button>
  </div>
	%s
</div>

<script>
  setTimeout(() => { htmx.trigger('#shares-list', 'refresh'); }, 300);
  document.getElementById('upload-form').reset();
  document.getElementById('drop-title').textContent = 'Drag your file here';
  document.getElementById('dropzone').classList.remove('has-file');
  document.getElementById('upload-btn').disabled = true;
</script>
`,
		template.HTMLEscapeString(originalName),
		storage.HumanSize(sizeBytes),
		days,
		template.HTMLEscapeString(downloadURL),
		func() string {
			if !a.cfg.EnableSHA256 {
				return ""
			}
			return `<p class="sha-pending-note">SHA-256 in progress: it will appear in the dashboard when ready.</p>`
		}(),
	)
}

// POST /share/{token}/compute-sha256 — compute SHA-256 for legacy/missing hashes
func (a *App) handleComputeShareSHA256(w http.ResponseWriter, r *http.Request, token string) {
	username := a.getUsername(r)

	share, err := a.db.GetShareByToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", 404)
			return
		}
		http.Error(w, "database error", 500)
		return
	}
	if share.Uploader != username {
		http.Error(w, "forbidden", 403)
		return
	}

	// If hash already exists, just return the row
	if share.SHA256 != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<p class="sha-chip">Copy SHA-256: <code>%s</code></p>`, template.HTMLEscapeString(share.SHA256))
		return
	}

	// Compute hash asynchronously and return pending state
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="badge badge-pending" data-sha256-pending="true">In progress</span>
<script>
  setTimeout(() => { htmx.trigger('#share-%s', 'refresh'); }, 2500);
</script>`, template.HTMLEscapeString(token))

	// Trigger async computation
	go func(token, path string) {
		hash, err := storage.ComputeSHA256(path)
		if err != nil {
			logger.Error("compute sha256 on-demand (%s): %v", token, err)
			return
		}
		if err := a.db.SetShareSHA256(token, hash); err != nil {
			logger.Error("store sha256 on-demand (%s): %v", token, err)
		}
	}(share.Token, share.FilePath)
}

// DELETE /share/{token}
func (a *App) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)
	token := strings.TrimPrefix(r.URL.Path, "/share/")
	if token == "" {
		http.Error(w, "missing token", 400)
		return
	}

	share, err := a.db.GetShareByToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", 404)
			return
		}
		http.Error(w, "database error", 500)
		return
	}
	if share.Uploader != username {
		http.Error(w, "forbidden", 403)
		return
	}

	storage.DeleteFile(share.FilePath)
	if err := a.db.DeleteShare(token); err != nil {
		logger.Error("delete share: %v", err)
		http.Error(w, "database error", 500)
		return
	}

	// HTMX: return empty 200 to remove the row
	w.WriteHeader(http.StatusOK)
}

// GET /shares-list  (HTMX partial refresh)
func (a *App) handleSharesList(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)
	shares, _ := a.db.ListSharesByUser(username)

	baseURL := a.publicBaseURL(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t := a.templates["dashboard"]
	locale := a.requestLocale(r)
	localized, cloneErr := t.Clone()
	if cloneErr != nil {
		logger.Error("clone dashboard template: %v", cloneErr)
		http.Error(w, "template error", 500)
		return
	}
	localized = localized.Funcs(a.localizedTemplateFuncs(locale))
	err := localized.ExecuteTemplate(w, "shares-list", map[string]any{
		"Shares":    shares,
		"BaseURL":   baseURL,
		"EnableSHA": a.cfg.EnableSHA256,
	})
	if err != nil {
		logger.Error("render shares-list: %v", err)
	}
}

// GET /d/{token}  — public download landing page
func (a *App) handleDownloadPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/d/")
	share, err := a.db.GetShareByToken(token)

	type dlData struct {
		Share          *database.Share
		Expired        bool
		Error          string
		HumanSz        string
		SHA256         string
		Version        string
		BrandName      string
		BrandLogo      string
		RequiresAuth   bool
		PasswordFailed bool
	}
	data := dlData{
		Version:   AppVersion,
		BrandName: a.cfg.BrandName,
		BrandLogo: a.cfg.BrandLogoPath,
	}

	locale := a.requestLocale(r)
	if err != nil {
		if err == sql.ErrNoRows {
			data.Error = i18n.T(locale, "download.link_not_found")
		} else {
			logger.Error("get share: %v", err)
			data.Error = i18n.T(locale, "download.internal_error")
		}
		a.render(w, r, "download", data)
		return
	}

	data.Share = share
	data.HumanSz = storage.HumanSize(share.SizeBytes)
	data.SHA256 = share.SHA256
	if time.Now().After(share.ExpiresAt) {
		data.Expired = true
	} else if share.MaxDownloads > 0 && share.Downloaded >= share.MaxDownloads {
		data.Expired = true
	}

	if share.PasswordHash != "" && !data.Expired {
		sess, err := a.store.Get(r, sessionName)
		if err != nil {
			logger.Error("get session: %v", err)
			sess, _ = a.store.New(r, sessionName)
		}

		unlocked, _ := sess.Values["ul_"+token].(bool)

		if r.Method == http.MethodPost {
			pass := r.FormValue("password")
			if err := bcrypt.CompareHashAndPassword([]byte(share.PasswordHash), []byte(pass)); err == nil {
				sess.Values["ul_"+token] = true
				sess.Options = &sessions.Options{
					Path:     "/",
					MaxAge:   86400 * 7, // 7 days
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   shouldUseSecureCookie(r, a.cfg), // Auto-detect da X-Forwarded-Proto o config
				}

				http.Redirect(w, r, "/d/"+token, http.StatusSeeOther)
				return
			} else {
				data.PasswordFailed = true
			}
		}

		if !unlocked {
			data.RequiresAuth = true
		}
	}

	a.render(w, r, "download", data)
}

// GET /download/{token}  — actual file streaming
func (a *App) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/download/")
	share, err := a.db.GetShareByToken(token)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if time.Now().After(share.ExpiresAt) || (share.MaxDownloads > 0 && share.Downloaded >= share.MaxDownloads) {
		http.Error(w, "expired link", 410)
		return
	}

	if share.PasswordHash != "" {
		sess, _ := a.store.Get(r, sessionName)
		unlocked, _ := sess.Values["ul_"+token].(bool)
		if !unlocked {
			http.Error(w, "forbidden", 403)
			return
		}
	}

	if err := storage.ServeFile(w, r, share.FilePath, share.OriginalName); err != nil {
		logger.Error("serve file: %v", err)
	}

	a.db.IncrementDownload(token)

	// Burn after reading logic
	if share.MaxDownloads > 0 && share.Downloaded+1 >= share.MaxDownloads {
		go func() {
			storage.DeleteFile(share.FilePath)
			a.db.DeleteShare(token)
			logger.Info("burned share %s after %d downloads", token, share.Downloaded+1)
		}()
	}
}

// ── Chunked upload API handlers ───────────────────────────────────────────────

type jsonObj = map[string]any

func jsonResponse(w http.ResponseWriter, status int, obj jsonObj) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		logger.Error("json encode: %v", err)
	}
}

// jsonDecode decodes the JSON body of r into v (1 MB limit).
func jsonDecode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

// POST /api/check-upload
// Body (JSON): { "filename": "...", "size": 12345678 }
func (a *App) handleCheckUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := a.getUsername(r)

	var req struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}
	if err := jsonDecode(r, &req); err != nil {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"ok": false, "reason": "bad_request"})
		return
	}

	maxBytes := a.cfg.MaxUploadSizeMB * 1024 * 1024
	if req.Size > maxBytes {
		jsonResponse(w, http.StatusRequestEntityTooLarge, jsonObj{"ok": false, "reason": "file_too_large"})
		return
	}

	if a.cfg.UserQuotaMB > 0 {
		usedBytes, err := a.db.GetUserTotalBytes(username)
		if err != nil {
			logger.Error("check-upload quota: %v", err)
			jsonResponse(w, http.StatusInternalServerError, jsonObj{"ok": false, "reason": "server_error"})
			return
		}
		if usedBytes+req.Size > a.cfg.UserQuotaMB*1024*1024 {
			jsonResponse(w, http.StatusForbidden, jsonObj{"ok": false, "reason": "quota_exceeded"})
			return
		}
	}

	jsonResponse(w, http.StatusOK, jsonObj{"ok": true})
}

// POST /api/upload/init
// Body (JSON): { "filename":"...", "size":12345678, "total_chunks":10, "chunk_size":10485760, "days":7, "password":"", "max_downloads":0 }
func (a *App) handleUploadInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := a.getUsername(r)

	var req struct {
		Filename     string `json:"filename"`
		Size         int64  `json:"size"`
		TotalChunks  int    `json:"total_chunks"`
		ChunkSize    int64  `json:"chunk_size"`
		Days         int    `json:"days"`
		Password     string `json:"password"`
		MaxDownloads int    `json:"max_downloads"`
	}
	if err := jsonDecode(r, &req); err != nil {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "bad_request"})
		return
	}

	// Sanitize filename
	req.Filename = filepath.Base(req.Filename)
	if req.Filename == "." || req.Filename == "" {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "invalid_filename"})
		return
	}

	if req.Days < 1 || req.Days > a.cfg.MaxGlobalDays {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "invalid_days"})
		return
	}
	if req.MaxDownloads < 0 {
		req.MaxDownloads = 0
	}
	if req.TotalChunks < 1 || req.ChunkSize < 1 {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "invalid_chunk_params"})
		return
	}

	// Limit concurrent upload sessions per user
	count, err := a.db.CountActiveUploadSessionsByUser(username)
	if err != nil {
		logger.Error("upload init count sessions: %v", err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
		return
	}
	if count >= a.cfg.MaxUploadSessionsPerUser {
		jsonResponse(w, http.StatusTooManyRequests, jsonObj{"error": "too_many_sessions"})
		return
	}

	if a.cfg.UserQuotaMB > 0 {
		usedBytes, err := a.db.GetUserTotalBytes(username)
		if err != nil {
			logger.Error("upload init quota: %v", err)
			jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
			return
		}
		if usedBytes+req.Size > a.cfg.UserQuotaMB*1024*1024 {
			jsonResponse(w, http.StatusForbidden, jsonObj{"error": "quota_exceeded"})
			return
		}
	}

	var passwordHash string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("bcrypt init: %v", err)
			jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
			return
		}
		passwordHash = string(hash)
	}

	sessionToken := uuid.New().String()
	expiresAt := time.Now().UTC().Add(time.Duration(a.cfg.UploadSessionTTLHours) * time.Hour)

	if err := os.MkdirAll(storage.ChunkDir(a.cfg.UploadDir, sessionToken), 0750); err != nil {
		logger.Error("upload init mkdir: %v", err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
		return
	}

	_, err = a.db.CreateUploadSession(sessionToken, username, req.Filename, req.Size, req.TotalChunks, req.ChunkSize, expiresAt, req.Days, passwordHash, req.MaxDownloads)
	if err != nil {
		logger.Error("upload init db: %v", err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
		return
	}

	jsonResponse(w, http.StatusOK, jsonObj{
		"session":      sessionToken,
		"chunk_size":   req.ChunkSize,
		"total_chunks": req.TotalChunks,
	})
}

// GET /api/upload/{session}/status
func (a *App) handleUploadStatus(w http.ResponseWriter, r *http.Request, sessionToken string) {
	username := a.getUsername(r)

	sess, err := a.db.GetUploadSession(sessionToken)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, jsonObj{"error": "session_not_found"})
		return
	}
	if sess.Uploader != username {
		jsonResponse(w, http.StatusForbidden, jsonObj{"error": "forbidden"})
		return
	}

	done := sess.DoneChunkList()
	var bytesReceived int64
	for _, idx := range done {
		if idx < sess.TotalChunks-1 {
			bytesReceived += sess.ChunkSize
		} else {
			remainder := sess.TotalSize - sess.ChunkSize*int64(sess.TotalChunks-1)
			if remainder > 0 {
				bytesReceived += remainder
			} else {
				bytesReceived += sess.ChunkSize
			}
		}
	}

	jsonResponse(w, http.StatusOK, jsonObj{
		"session":         sessionToken,
		"total_chunks":    sess.TotalChunks,
		"chunk_size":      sess.ChunkSize,
		"bytes_received":  bytesReceived,
		"chunks_received": done,
	})
}

// POST /api/upload/{session}/chunk/{index}
// Body: raw chunk bytes (Content-Type: application/octet-stream)
func (a *App) handleUploadChunk(w http.ResponseWriter, r *http.Request, sessionToken string, chunkIndex int) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := a.getUsername(r)

	sess, err := a.db.GetUploadSession(sessionToken)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, jsonObj{"error": "session_not_found"})
		return
	}
	if sess.Uploader != username {
		jsonResponse(w, http.StatusForbidden, jsonObj{"error": "forbidden"})
		return
	}
	if chunkIndex < 0 || chunkIndex >= sess.TotalChunks {
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "invalid_chunk_index"})
		return
	}

	expectedSize := sess.ChunkSize
	if chunkIndex == sess.TotalChunks-1 {
		remainder := sess.TotalSize - sess.ChunkSize*int64(sess.TotalChunks-1)
		if remainder > 0 {
			expectedSize = remainder
		}
	}

	limited := http.MaxBytesReader(w, r.Body, expectedSize+1)

	bytesWritten, err := storage.SaveChunk(a.cfg.UploadDir, sessionToken, chunkIndex, limited)
	if err != nil {
		logger.Error("save chunk %s[%d]: %v", sessionToken, chunkIndex, err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "write_error"})
		return
	}
	if bytesWritten > expectedSize {
		os.Remove(storage.ChunkFilePath(a.cfg.UploadDir, sessionToken, chunkIndex))
		jsonResponse(w, http.StatusBadRequest, jsonObj{"error": "chunk_too_large"})
		return
	}

	if err := a.db.MarkChunkReceived(sessionToken, chunkIndex); err != nil {
		logger.Error("mark chunk %s[%d]: %v", sessionToken, chunkIndex, err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "db_error"})
		return
	}

	jsonResponse(w, http.StatusOK, jsonObj{
		"index":         chunkIndex,
		"bytes_written": bytesWritten,
	})
}

// POST /api/upload/{session}/complete
func (a *App) handleUploadComplete(w http.ResponseWriter, r *http.Request, sessionToken string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := a.getUsername(r)

	var reqPayload struct {
		ShareEmail string `json:"share_email"`
	}
	// It's acceptable if jsondecode fails, e.g. empty body
	_ = jsonDecode(r, &reqPayload)
	shareEmail := strings.TrimSpace(reqPayload.ShareEmail)

	var userEmail string
	var userPassword string
	if a.cfg.SMTPUserAuth && shareEmail != "" {
		sess, _ := a.store.Get(r, sessionName)
		userEmail = username
		if pwd, ok := sess.Values["smtp_password"].(string); ok {
			userPassword = pwd
		}
	}

	sess, err := a.db.GetUploadSession(sessionToken)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, jsonObj{"error": "session_not_found"})
		return
	}
	if sess.Uploader != username {
		jsonResponse(w, http.StatusForbidden, jsonObj{"error": "forbidden"})
		return
	}

	done := sess.DoneChunkList()
	if len(done) != sess.TotalChunks {
		jsonResponse(w, http.StatusConflict, jsonObj{"error": "missing_chunks"})
		return
	}

	if err := os.MkdirAll(a.cfg.UploadDir, 0750); err != nil {
		logger.Error("complete mkdir: %v", err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
		return
	}

	destDir := filepath.Join(a.cfg.UploadDir, uuid.New().String())
	if err := os.MkdirAll(destDir, 0750); err != nil {
		logger.Error("complete mkdir dest: %v", err)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "server_error"})
		return
	}
	destPath := filepath.Join(destDir, sess.OriginalName)

	if err := storage.ComposeChunks(a.cfg.UploadDir, sessionToken, sess.TotalChunks, destPath); err != nil {
		logger.Error("compose chunks %s: %v", sessionToken, err)
		os.RemoveAll(destDir)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "compose_error"})
		return
	}

	shareToken := uuid.New().String()
	expiresAt := time.Now().UTC().Add(time.Duration(sess.Days) * 24 * time.Hour)

	share, err := a.db.CreateShare(shareToken, destPath, sess.OriginalName, sess.TotalSize, username, expiresAt, "", sess.PasswordHash, sess.MaxDownloads)
	if err != nil {
		logger.Error("complete create share %s: %v", sessionToken, err)
		os.RemoveAll(destDir)
		jsonResponse(w, http.StatusInternalServerError, jsonObj{"error": "db_error"})
		return
	}

	logger.Info("User %s completed chunked upload of %s (size: %s, token: %s)", username, sess.OriginalName, storage.HumanSize(sess.TotalSize), shareToken)

	if a.cfg.EnableSHA256 {
		go func(token, path string) {
			hash, err := storage.ComputeSHA256(path)
			if err != nil {
				logger.Error("compute sha256 async chunked (%s): %v", token, err)
				return
			}
			if err := a.db.SetShareSHA256(token, hash); err != nil {
				logger.Error("store sha256 async chunked (%s): %v", token, err)
			}
		}(shareToken, destPath)
	}

	// Cleanup chunks (best-effort)
	if err := storage.CleanupChunkDir(a.cfg.UploadDir, sessionToken); err != nil {
		logger.Warn("cleanup chunks %s: %v", sessionToken, err)
	}
	if err := a.db.DeleteUploadSession(sessionToken); err != nil {
		logger.Warn("delete upload session %s: %v", sessionToken, err)
	}

	downloadURL := fmt.Sprintf("%s/d/%s", a.publicBaseURL(r), share.Token)

	if shareEmail != "" {
		go func() {
			err := email.SendShareEmail(shareEmail, downloadURL, sess.OriginalName, sess.Days, sess.PasswordHash != "", username, a.cfg, userEmail, userPassword)
			if err != nil {
				logger.Error("failed to send chunked share email to %s: %v", shareEmail, err)
			} else {
				logger.Debug("chunked share email successfully sent to %s", shareEmail)
			}
		}()
	}

	jsonResponse(w, http.StatusOK, jsonObj{
		"token":        shareToken,
		"download_url": downloadURL,
		"filename":     sess.OriginalName,
		"size":         sess.TotalSize,
		"days":         sess.Days,
	})
}

// DELETE /api/upload/{session}
func (a *App) handleUploadAbort(w http.ResponseWriter, r *http.Request, sessionToken string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := a.getUsername(r)

	sess, err := a.db.GetUploadSession(sessionToken)
	if err != nil {
		// Already deleted or never existed — treat as success
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if sess.Uploader != username {
		jsonResponse(w, http.StatusForbidden, jsonObj{"error": "forbidden"})
		return
	}

	if err := storage.CleanupChunkDir(a.cfg.UploadDir, sessionToken); err != nil {
		logger.Warn("abort cleanup %s: %v", sessionToken, err)
	}
	if err := a.db.DeleteUploadSession(sessionToken); err != nil {
		logger.Warn("abort delete session %s: %v", sessionToken, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Cleanup goroutine ─────────────────────────────────────────────────────────

func (a *App) startCleanupJob() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			// Clean up expired shares
			expired, err := a.db.GetExpiredShares()
			if err != nil {
				logger.Error("cleanup: get expired: %v", err)
			} else {
				for _, s := range expired {
					if err := storage.DeleteFile(s.FilePath); err != nil {
						logger.Error("cleanup: delete file %s: %v", s.FilePath, err)
					}
					if err := a.db.DeleteShare(s.Token); err != nil {
						logger.Error("cleanup: delete share %s: %v", s.Token, err)
					} else {
						logger.Info("cleanup: removed expired share %s (%s)", s.Token, s.OriginalName)
					}
				}
			}
			// Clean up stale upload sessions
			stale, err := a.db.GetStaleUploadSessions()
			if err != nil {
				logger.Error("cleanup: get stale sessions: %v", err)
			} else {
				for _, s := range stale {
					if err := storage.CleanupChunkDir(a.cfg.UploadDir, s.SessionToken); err != nil {
						logger.Error("cleanup: remove chunks for session %s: %v", s.SessionToken, err)
					}
					if err := a.db.DeleteUploadSession(s.SessionToken); err != nil {
						logger.Error("cleanup: delete stale session %s: %v", s.SessionToken, err)
					} else {
						logger.Info("cleanup: removed stale upload session %s (%s)", s.SessionToken, s.OriginalName)
					}
				}
			}
		}
	}()
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := config.Load()
	logger.Init(cfg.LogLevel)

	// Ensure data directories exist
	if err := os.MkdirAll(cfg.UploadDir, 0750); err != nil {
		logger.Error("create upload dir: %v", err)
		os.Exit(1)
	}
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		logger.Error("create db dir: %v", err)
		os.Exit(1)
	}

	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		logger.Error("init db: %v", err)
		os.Exit(1)
	}

	// ── Cookie store con crittografia dual-key ──────────────────────────────────
	// Chiave 1: HMAC authentication (64 bytes)
	// Chiave 2: AES encryption (32 bytes)
	authKey := []byte(cfg.SessionSecret)
	if len(authKey) < 32 {
		// Pad to minimum 32 bytes if SESSION_SECRET is too short
		authKey = append(authKey, make([]byte, 32-len(authKey))...)
	}
	// Derive encryption key from session secret
	encKey := sha256.Sum256([]byte(cfg.SessionSecret + "-encryption"))

	app := &App{
		cfg:   cfg,
		db:    db,
		store: sessions.NewCookieStore(authKey, encKey[:]),
	}

	// Detect web directory (supports both dev and container layout)
	webDir := "web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		webDir = "/app/web"
	}
	if err := app.loadTemplates(filepath.Join(webDir, "templates")); err != nil {
		logger.Error("load templates: %v", err)
		os.Exit(1)
	}

	app.startCleanupJob()

	// Detect web directory (supports both dev and container layout)
	staticDir := filepath.Join(webDir, "static")

	// ── Unified mux (porta APP_PORT, default 8080) ─────────────────────────────
	mux := http.NewServeMux()

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Corporate Branding Logo (served from /data)
	mux.HandleFunc("/brand-logo", app.handleBrandLogo)

	// Public routes (Download)
	mux.HandleFunc("/d/", app.handleDownloadPage)
	mux.HandleFunc("/download/", app.handleDownloadFile)

	// Admin/Authenticated routes
	mux.HandleFunc("/", app.handleRoot)
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			app.handleLoginPage(w, r)
		case http.MethodPost:
			app.handleLogin(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/logout", app.handleLogout)
	mux.HandleFunc("/dashboard", app.requireAuth(app.handleDashboard))
	mux.HandleFunc("/admin", app.requireAuth(app.requireAdmin(app.handleAdminDashboard)))
	mux.HandleFunc("/upload", app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		app.handleUpload(w, r)
	}))
	mux.HandleFunc("/share/", app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/share/")
		parts := strings.Split(path, "/")
		token := parts[0]

		// POST /share/{token}/compute-sha256
		if len(parts) > 1 && parts[1] == "compute-sha256" && r.Method == http.MethodPost {
			app.handleComputeShareSHA256(w, r, token)
			return
		}

		// DELETE /share/{token}
		if r.Method == http.MethodDelete {
			app.handleDeleteShare(w, r)
			return
		}

		http.Error(w, "method not allowed", 405)
	}))
	mux.HandleFunc("/shares-list", app.requireAuth(app.handleSharesList))

	// Chunked upload API routes (all require authentication)
	mux.HandleFunc("/api/check-upload", app.requireAuth(app.handleCheckUpload))
	mux.HandleFunc("/api/upload/init", app.requireAuth(app.handleUploadInit))
	mux.HandleFunc("/api/upload/", app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		// Path: /api/upload/{session}/status
		//       /api/upload/{session}/chunk/{index}
		//       /api/upload/{session}/complete
		//       DELETE /api/upload/{session}
		path := strings.TrimPrefix(r.URL.Path, "/api/upload/")
		parts := strings.Split(path, "/")
		if len(parts) < 1 || parts[0] == "" {
			http.Error(w, "not found", 404)
			return
		}
		session := parts[0]
		// DELETE /api/upload/{session} — abort
		if r.Method == http.MethodDelete && len(parts) == 1 {
			app.handleUploadAbort(w, r, session)
			return
		}
		if len(parts) < 2 {
			http.Error(w, "not found", 404)
			return
		}
		action := parts[1]
		switch {
		case action == "status" && r.Method == http.MethodGet:
			app.handleUploadStatus(w, r, session)
		case action == "complete" && r.Method == http.MethodPost:
			app.handleUploadComplete(w, r, session)
		case action == "chunk" && len(parts) >= 3 && r.Method == http.MethodPost:
			var idx int
			if _, err := fmt.Sscanf(parts[2], "%d", &idx); err != nil {
				http.Error(w, "invalid chunk index", 400)
				return
			}
			app.handleUploadChunk(w, r, session, idx)
		default:
			http.Error(w, "not found", 404)
		}
	}))

	// Admin Panel route
	mux.HandleFunc("/admin", app.requireAuth(app.requireAdmin(app.handleAdminDashboard)))

	if cfg.LDAPHost == "mock" {
		log.Printf("[WARN] RUNNING IN MOCK MODE - any credentials accepted")
	}

	addr := ":" + cfg.Port
	log.Printf("GoPulley started on http://0.0.0.0%s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
