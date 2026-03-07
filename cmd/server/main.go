package main

import (
	"database/sql"
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
	// Il logo è in /data (es. /data/my-logo.png)
	logoPath := filepath.Join(filepath.Dir(a.cfg.DBPath), a.cfg.BrandLogoPath)
	http.ServeFile(w, r, logoPath)
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
		"fmtDate": func(t time.Time) string {
			return t.Format("02 Jan 2006 15:04")
		},
		"daysLeft": func(t time.Time) string {
			d := time.Until(t)
			if d <= 0 {
				return "scaduto"
			}
			if d < 24*time.Hour {
				hours := int(d.Hours())
				if hours < 1 {
					hours = 1
				}
				return fmt.Sprintf("%dh", hours)
			}
			days := int(d.Hours() / 24)
			if days == 1 {
				return "1 giorno"
			}
			return fmt.Sprintf("%d giorni", days)
		},
		"isExpiringSoon": func(t time.Time) bool {
			d := time.Until(t)
			return d > 0 && d < 24*time.Hour
		},
		"isExpired": func(t time.Time) bool {
			return time.Now().After(t)
		},
		"daysAgo": func(t time.Time) string {
			days := int(time.Since(t).Hours() / 24)
			if days == 0 {
				return "oggi"
			}
			if days == 1 {
				return "ieri"
			}
			return fmt.Sprintf("%d giorni fa", days)
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

func (a *App) render(w http.ResponseWriter, name string, data any) {
	tmpl, ok := a.templates[name]
	if !ok {
		http.Error(w, "template not found", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name+".html", data); err != nil {
		log.Printf("render %s: %v", name, err)
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
	a.render(w, "login", map[string]any{
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

	log.Printf("📥 Ricevuta richiesta di login per: %s", username)

	ok, isAdmin, err := auth.Authenticate(username, password, a.cfg)
	if err != nil {
		log.Printf("auth error for %s: %v", username, err)
		w.Header().Set("HX-Reswap", "outerHTML")
		a.renderError(w, "Errore di connessione al server LDAP. Riprova.")
		return
	}
	if !ok {
		log.Printf("login failed for %s: invalid credentials or not in required group", username)
		w.Header().Set("HX-Reswap", "outerHTML")
		// HTMX drops 4xx/5xx by default. We return 200 so the error message is displayed.
		a.renderError(w, "Credenziali non valide o utente non autorizzato.")
		return
	}

	sess, _ := a.store.Get(r, sessionName)
	sess.Values["username"] = username
	sess.Values["is_admin"] = isAdmin
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if err := sess.Save(r, w); err != nil {
		log.Printf("session save: %v", err)
	}

	// HTMX redirect
	w.Header().Set("HX-Redirect", "/dashboard")
	w.WriteHeader(http.StatusOK)
}

func (a *App) renderError(w http.ResponseWriter, msg string) {
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
		log.Printf("list shares: %v", err)
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
			log.Printf("get quota bytes error: %v", err)
		}
	}

	baseURL := a.publicBaseURL(r)

	a.render(w, "dashboard", dashData{
		Username:     username,
		IsAdmin:      isAdmin,
		Shares:       shares,
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
		log.Printf("list all shares error: %v", err)
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
		log.Printf("disk usage error for %s: %v", dataPath, err)
	}

	a.render(w, "admin", adminDashData{
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
		http.Error(w, "file troppo grande o form non valido", http.StatusBadRequest)
		return
	}

	// Days validation
	days := 0
	fmt.Sscanf(r.FormValue("days"), "%d", &days)
	if days < 1 || days > a.cfg.MaxGlobalDays {
		http.Error(w, fmt.Sprintf("durata non valida (1–%d giorni)", a.cfg.MaxGlobalDays), http.StatusBadRequest)
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
			log.Printf("bcrypt error: %v", err)
			http.Error(w, "errore server (password)", 500)
			return
		}
		passwordHash = string(hash)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "nessun file ricevuto", http.StatusBadRequest)
		return
	}
	file.Close()

	// Quota check
	if a.cfg.UserQuotaMB > 0 {
		usedBytes, err := a.db.GetUserTotalBytes(username)
		if err != nil {
			log.Printf("quota check error: %v", err)
			http.Error(w, "errore server (quota)", 500)
			return
		}
		if usedBytes+header.Size > a.cfg.UserQuotaMB*1024*1024 {
			http.Error(w, "quota spazio superata", http.StatusForbidden)
			return
		}
	}

	// Ensure upload dir exists
	if err := os.MkdirAll(a.cfg.UploadDir, 0750); err != nil {
		log.Printf("mkdir uploads: %v", err)
		http.Error(w, "errore server", 500)
		return
	}

	filePath, originalName, sizeBytes, err := storage.SaveFile(header, a.cfg.UploadDir)
	if err != nil {
		log.Printf("save file: %v", err)
		http.Error(w, "errore salvataggio file", 500)
		return
	}

	token := uuid.New().String()
	expiresAt := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)

	share, err := a.db.CreateShare(token, filePath, originalName, sizeBytes, username, expiresAt, "", passwordHash, maxDownloads)
	if err != nil {
		log.Printf("create share: %v", err)
		storage.DeleteFile(filePath)
		http.Error(w, "errore database", 500)
		return
	}

	if a.cfg.EnableSHA256 {
		// Compute hash asynchronously to avoid blocking the upload response on large files.
		go func(token, path string) {
			hash, err := storage.ComputeSHA256(path)
			if err != nil {
				log.Printf("compute sha256 async (%s): %v", token, err)
				return
			}
			if err := a.db.SetShareSHA256(token, hash); err != nil {
				log.Printf("store sha256 async (%s): %v", token, err)
			}
		}(share.Token, filePath)
	}

	downloadURL := fmt.Sprintf("%s/d/%s", a.publicBaseURL(r), share.Token)

	// Return HTMX response: a success card
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
<div id="upload-result" class="upload-result success" role="alert">
  <div class="result-inner">
    <span class="result-icon">✓</span>
    <div class="result-content">
      <strong>%s</strong> caricato con successo!<br>
      <small>%s &middot; scade tra %d giorni</small>
    </div>
  </div>
  <div class="link-box">
    <input type="text" value="%s" readonly id="share-link" />
    <button class="btn btn-copy" onclick="copyLink()">Copia link</button>
  </div>
	%s
</div>

<script>
  setTimeout(() => { htmx.trigger('#shares-list', 'refresh'); }, 300);
  document.getElementById('upload-form').reset();
  document.getElementById('drop-title').textContent = 'Trascina qui il file';
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
			return `<p class="sha-pending-note">SHA-256 in lavorazione: comparira nella dashboard appena pronto.</p>`
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
		http.Error(w, "errore database", 500)
		return
	}
	if share.Uploader != username {
		http.Error(w, "forbidden", 403)
		return
	}

	// If hash already exists, just return the row
	if share.SHA256 != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<p class="sha-chip">Copia SHA-256: <code>%s</code></p>`, template.HTMLEscapeString(share.SHA256))
		return
	}

	// Compute hash asynchronously and return pending state
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="badge badge-pending" data-sha256-pending="true">In lavorazione</span>
<script>
  setTimeout(() => { htmx.trigger('#share-%s', 'refresh'); }, 2500);
</script>`, template.HTMLEscapeString(token))

	// Trigger async computation
	go func(token, path string) {
		hash, err := storage.ComputeSHA256(path)
		if err != nil {
			log.Printf("compute sha256 on-demand (%s): %v", token, err)
			return
		}
		if err := a.db.SetShareSHA256(token, hash); err != nil {
			log.Printf("store sha256 on-demand (%s): %v", token, err)
		}
	}(share.Token, share.FilePath)
}

// DELETE /share/{token}
func (a *App) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	username := a.getUsername(r)
	token := strings.TrimPrefix(r.URL.Path, "/share/")
	if token == "" {
		http.Error(w, "token mancante", 400)
		return
	}

	share, err := a.db.GetShareByToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", 404)
			return
		}
		http.Error(w, "errore database", 500)
		return
	}
	if share.Uploader != username {
		http.Error(w, "forbidden", 403)
		return
	}

	storage.DeleteFile(share.FilePath)
	if err := a.db.DeleteShare(token); err != nil {
		log.Printf("delete share: %v", err)
		http.Error(w, "errore database", 500)
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
	err := a.templates["dashboard"].ExecuteTemplate(w, "shares-list", map[string]any{
		"Shares":    shares,
		"BaseURL":   baseURL,
		"EnableSHA": a.cfg.EnableSHA256,
	})
	if err != nil {
		log.Printf("render shares-list: %v", err)
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

	if err != nil {
		if err == sql.ErrNoRows {
			data.Error = "Link non trovato o già eliminato."
		} else {
			log.Printf("get share: %v", err)
			data.Error = "Errore interno. Riprova più tardi."
		}
		a.render(w, "download", data)
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
			log.Printf("get session: %v", err)
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
					Secure:   false,
				}

				if err := sess.Save(r, w); err != nil {
					log.Printf("save session: %v", err)
					http.Error(w, "errore server", 500)
					return
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

	a.render(w, "download", data)
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
		http.Error(w, "link scaduto", 410)
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
		log.Printf("serve file: %v", err)
	}

	a.db.IncrementDownload(token)

	// Burn after reading logic
	if share.MaxDownloads > 0 && share.Downloaded+1 >= share.MaxDownloads {
		go func() {
			storage.DeleteFile(share.FilePath)
			a.db.DeleteShare(token)
			log.Printf("burned share %s after %d downloads", token, share.Downloaded+1)
		}()
	}
}

// ── Cleanup goroutine ─────────────────────────────────────────────────────────

func (a *App) startCleanupJob() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			expired, err := a.db.GetExpiredShares()
			if err != nil {
				log.Printf("cleanup: get expired: %v", err)
				continue
			}
			for _, s := range expired {
				if err := storage.DeleteFile(s.FilePath); err != nil {
					log.Printf("cleanup: delete file %s: %v", s.FilePath, err)
				}
				if err := a.db.DeleteShare(s.Token); err != nil {
					log.Printf("cleanup: delete share %s: %v", s.Token, err)
				} else {
					log.Printf("cleanup: removed expired share %s (%s)", s.Token, s.OriginalName)
				}
			}
		}
	}()
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := config.Load()

	// Ensure data directories exist
	if err := os.MkdirAll(cfg.UploadDir, 0750); err != nil {
		log.Fatalf("create upload dir: %v", err)
	}
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		log.Fatalf("create db dir: %v", err)
	}

	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}

	app := &App{
		cfg:   cfg,
		db:    db,
		store: sessions.NewCookieStore([]byte(cfg.SessionSecret)),
	}

	// Detect web directory (supports both dev and container layout)
	webDir := "web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		webDir = "/app/web"
	}
	if err := app.loadTemplates(filepath.Join(webDir, "templates")); err != nil {
		log.Fatalf("load templates: %v", err)
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
