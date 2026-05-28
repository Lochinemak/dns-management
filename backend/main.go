package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAddr      = ":8080"
	defaultJWTSecret = "dev-change-me"
)

var (
	prefixPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	namePattern   = regexp.MustCompile(`^(?:@|[a-zA-Z0-9_*](?:[a-zA-Z0-9_*.-]{0,251}[a-zA-Z0-9_*])?)$`)
	recordTypes   = []string{"A", "AAAA", "CNAME", "TXT", "MX", "NS"}
	statuses      = []string{"pending", "active", "rejected", "suspended"}
	ttls          = []int{1, 60, 3600}
)

type contextKey string

const userContextKey contextKey = "user"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Nickname  string    `json:"nickname"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type Domain struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ZoneID         string    `json:"zoneId"`
	Enabled        bool      `json:"enabled"`
	TokenMasked    string    `json:"tokenMasked"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	EncryptedToken string    `json:"-"`
}

type Subdomain struct {
	ID           string     `json:"id"`
	OwnerID      string     `json:"ownerId"`
	OwnerEmail   string     `json:"ownerEmail,omitempty"`
	DomainID     string     `json:"domainId"`
	DomainName   string     `json:"domainName"`
	Prefix       string     `json:"prefix"`
	FullDomain   string     `json:"fullDomain"`
	Status       string     `json:"status"`
	RejectReason string     `json:"rejectReason,omitempty"`
	ReviewedBy   string     `json:"reviewedBy,omitempty"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type DNSRecord struct {
	ID                 string    `json:"id"`
	SubdomainID        string    `json:"subdomainId"`
	CloudflareRecordID string    `json:"cloudflareRecordId,omitempty"`
	Type               string    `json:"type"`
	Name               string    `json:"name"`
	Content            string    `json:"content"`
	TTL                int       `json:"ttl"`
	Proxied            bool      `json:"proxied"`
	CreatedAt          time.Time `json:"createdAt"`
}

type APIToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	TokenHash string    `json:"-"`
}

type tokenResponse struct {
	APIToken
	Token string `json:"token,omitempty"`
}

type Server struct {
	db          *sql.DB
	jwtSecret   []byte
	tokenKey    []byte
	frontendDir string
	http        *http.Client
	now         func() time.Time
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	if dsn == "" {
		log.Fatal("MYSQL_DSN is required, for example root:password@tcp(127.0.0.1:3306)/dns_management?parseTime=true&multiStatements=true")
	}
	db, err := sql.Open("mysql", ensureDSNOptions(dsn))
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}

	srv := NewServer(db, env("JWT_SECRET", defaultJWTSecret), env("TOKEN_ENCRYPTION_KEY", env("JWT_SECRET", defaultJWTSecret)))
	if err := srv.migrate(context.Background()); err != nil {
		log.Fatalf("migrate mysql: %v", err)
	}

	addr := env("ADDR", defaultAddr)
	log.Printf("dns management backend listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}

func NewServer(db *sql.DB, jwtSecret, tokenSecret string) *Server {
	sum := sha256.Sum256([]byte(tokenSecret))
	return &Server{
		db:          db,
		jwtSecret:   []byte(jwtSecret),
		tokenKey:    sum[:],
		frontendDir: env("FRONTEND_DIR", "public"),
		http:        &http.Client{Timeout: 8 * time.Second},
		now:         func() time.Time { return time.Now().UTC() },
	}
}

func (s *Server) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id VARCHAR(64) PRIMARY KEY,
			email VARCHAR(255) NOT NULL UNIQUE,
			nickname VARCHAR(255) NOT NULL,
			role ENUM('user','admin') NOT NULL DEFAULT 'user',
			password_hash VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS domains (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			zone_id VARCHAR(255) NOT NULL,
			api_token_encrypted TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS subdomains (
			id VARCHAR(64) PRIMARY KEY,
			owner_id VARCHAR(64) NOT NULL,
			domain_id VARCHAR(64) NOT NULL,
			prefix VARCHAR(63) NOT NULL,
			status ENUM('pending','active','rejected','suspended') NOT NULL DEFAULT 'pending',
			reject_reason TEXT NULL,
			reviewed_by VARCHAR(64) NULL,
			reviewed_at TIMESTAMP NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uniq_subdomain (domain_id, prefix),
			INDEX idx_subdomains_owner (owner_id),
			INDEX idx_subdomains_status (status),
			FOREIGN KEY (owner_id) REFERENCES users(id),
			FOREIGN KEY (domain_id) REFERENCES domains(id)
		)`,
		`CREATE TABLE IF NOT EXISTS dns_records (
			id VARCHAR(64) PRIMARY KEY,
			subdomain_id VARCHAR(64) NOT NULL,
			type VARCHAR(16) NOT NULL,
			name VARCHAR(255) NOT NULL,
			content TEXT NOT NULL,
			ttl INT NOT NULL,
			proxied BOOLEAN NOT NULL DEFAULT FALSE,
			cloudflare_record_id VARCHAR(255) NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_dns_records_subdomain (subdomain_id),
			FOREIGN KEY (subdomain_id) REFERENCES subdomains(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id VARCHAR(64) PRIMARY KEY,
			user_id VARCHAR(64) NOT NULL,
			name VARCHAR(255) NOT NULL,
			token_hash CHAR(64) NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_api_tokens_user (user_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.ensureColumn(ctx, "dns_records", "cloudflare_record_id", `ALTER TABLE dns_records ADD COLUMN cloudflare_record_id VARCHAR(255) NULL AFTER proxied`); err != nil {
		return err
	}
	if err := s.ensureIndex(ctx, "dns_records", "uniq_dns_records_cloudflare", `ALTER TABLE dns_records ADD UNIQUE KEY uniq_dns_records_cloudflare (cloudflare_record_id)`); err != nil {
		return err
	}
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.Handle("/api/v1/setup/status", s.withCORS(http.HandlerFunc(s.setupStatus)))
	mux.Handle("/api/v1/setup/admin", s.withCORS(http.HandlerFunc(s.setupAdmin)))
	mux.Handle("/api/v1/auth/login", s.withCORS(http.HandlerFunc(s.login)))
	mux.Handle("/api/v1/", s.withCORS(s.withAuth(http.HandlerFunc(s.api))))
	mux.Handle("/", http.HandlerFunc(s.frontend))
	return s.withCORS(mux)
}

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.frontendDir == "" {
		http.NotFound(w, r)
		return
	}

	cleanPath := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))
	filePath := filepath.Join(s.frontendDir, cleanPath)
	info, err := os.Stat(filePath)
	if err == nil && info.IsDir() {
		filePath = filepath.Join(filePath, "index.html")
		info, err = os.Stat(filePath)
	}
	if (err != nil || info.IsDir()) && cleanPath != "." && !strings.Contains(filepath.Base(cleanPath), ".") {
		htmlPath := filepath.Join(s.frontendDir, cleanPath+".html")
		if htmlInfo, htmlErr := os.Stat(htmlPath); htmlErr == nil && !htmlInfo.IsDir() {
			filePath = htmlPath
			info = htmlInfo
			err = nil
		}
	}
	if err != nil || info.IsDir() {
		if strings.HasPrefix(cleanPath, "_next") || strings.Contains(filepath.Base(cleanPath), ".") {
			http.NotFound(w, r)
			return
		}
		filePath = filepath.Join(s.frontendDir, "index.html")
		if _, err := os.Stat(filePath); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFile(w, r, filePath)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		user, err := s.userFromJWT(r.Context(), token)
		if err != nil {
			if user, ok := s.userByAPIToken(r.Context(), token); ok {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	parts := splitPath(path)
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	switch parts[0] {
	case "me":
		if r.Method == http.MethodGet && len(parts) == 1 {
			writeJSON(w, http.StatusOK, currentUser(r))
			return
		}
	case "auth":
		if len(parts) == 2 && parts[1] == "change-password" && r.Method == http.MethodPost {
			s.changePassword(w, r)
			return
		}
	case "domains":
		if len(parts) == 2 && parts[1] == "enabled" && r.Method == http.MethodGet {
			s.enabledDomains(w, r)
			return
		}
	case "subdomains":
		s.handleSubdomains(w, r, parts[1:])
		return
	case "tokens":
		s.handleTokens(w, r, parts[1:])
		return
	case "admin":
		s.handleAdmin(w, r, parts[1:])
		return
	case "dns-query":
		if r.Method == http.MethodGet && len(parts) == 1 {
			s.dnsQuery(w, r)
			return
		}
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	initialized, err := s.hasUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not inspect setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"initialized": initialized})
}

func (s *Server) setupAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	initialized, err := s.hasUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not inspect setup status")
		return
	}
	if initialized {
		writeError(w, http.StatusConflict, "system is already initialized")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Nickname string `json:"nickname"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	nickname := strings.TrimSpace(req.Nickname)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if nickname == "" {
		nickname = "Administrator"
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	user := User{ID: newID("usr"), Email: email, Nickname: nickname, Role: "admin", CreatedAt: s.now()}
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO users (id, email, nickname, role, password_hash) VALUES (?, ?, ?, 'admin', ?)`, user.ID, user.Email, user.Nickname, string(hash))
	if isDuplicate(err) {
		writeError(w, http.StatusConflict, "system is already initialized")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create admin")
		return
	}
	token, err := s.issueJWT(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "user": user})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, hash, err := s.userByEmail(r.Context(), req.Email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	token, err := s.issueJWT(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) hasUsers(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	user := currentUser(r)
	_, hash, err := s.userByEmail(r.Context(), user.Email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		writeError(w, http.StatusBadRequest, "old password is incorrect")
		return
	}
	next, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ? WHERE id = ?`, string(next), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) enabledDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, zone_id, enabled, created_at, updated_at FROM domains WHERE enabled = TRUE ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	defer rows.Close()
	domains, err := scanDomains(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not scan domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleSubdomains(w http.ResponseWriter, r *http.Request, parts []string) {
	user := currentUser(r)
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			s.listSubdomains(w, r, user, false)
		case http.MethodPost:
			s.createSubdomain(w, r, user)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.deleteSubdomain(w, r, user, parts[0])
		return
	}
	if len(parts) >= 2 && parts[1] == "records" {
		s.handleRecords(w, r, user, parts[0], parts[2:])
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) listSubdomains(w http.ResponseWriter, r *http.Request, user User, admin bool) {
	var rows *sql.Rows
	var err error
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	base := `SELECT s.id, s.owner_id, u.email, s.domain_id, d.name, s.prefix, s.status, COALESCE(s.reject_reason, ''), COALESCE(s.reviewed_by, ''), s.reviewed_at, s.created_at
		FROM subdomains s JOIN domains d ON d.id = s.domain_id JOIN users u ON u.id = s.owner_id`
	if admin {
		if status != "" {
			rows, err = s.db.QueryContext(r.Context(), base+` WHERE s.status = ? ORDER BY s.created_at DESC`, status)
		} else {
			rows, err = s.db.QueryContext(r.Context(), base+` ORDER BY s.created_at DESC`)
		}
	} else {
		rows, err = s.db.QueryContext(r.Context(), base+` WHERE s.owner_id = ? ORDER BY s.created_at DESC`, user.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list subdomains")
		return
	}
	defer rows.Close()
	subs, err := scanSubdomains(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not scan subdomains")
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

func (s *Server) createSubdomain(w http.ResponseWriter, r *http.Request, user User) {
	var req struct {
		DomainID string `json:"domainId"`
		Prefix   string `json:"prefix"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	prefix := strings.ToLower(strings.TrimSpace(req.Prefix))
	if !prefixPattern.MatchString(prefix) {
		writeError(w, http.StatusBadRequest, "invalid prefix")
		return
	}
	var domainName string
	err := s.db.QueryRowContext(r.Context(), `SELECT name FROM domains WHERE id = ? AND enabled = TRUE`, req.DomainID).Scan(&domainName)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "domain is not available")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load domain")
		return
	}
	id := newID("sub")
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO subdomains (id, owner_id, domain_id, prefix, status) VALUES (?, ?, ?, ?, 'pending')`, id, user.ID, req.DomainID, prefix)
	if isDuplicate(err) {
		writeError(w, http.StatusConflict, "subdomain is already taken")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create subdomain")
		return
	}
	writeJSON(w, http.StatusCreated, Subdomain{ID: id, OwnerID: user.ID, DomainID: req.DomainID, DomainName: domainName, Prefix: prefix, FullDomain: prefix + "." + domainName, Status: "pending", CreatedAt: s.now()})
}

func (s *Server) deleteSubdomain(w http.ResponseWriter, r *http.Request, user User, subID string) {
	var count int
	err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM subdomains WHERE id = ? AND (owner_id = ? OR ? = 'admin')`, subID, user.ID, user.Role).Scan(&count)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not inspect subdomain")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM dns_records WHERE subdomain_id = ?`, subID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete local DNS records")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM subdomains WHERE id = ? AND (owner_id = ? OR ? = 'admin')`, subID, user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete subdomain")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request, user User, subID string, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			s.listRecords(w, r, user, subID)
		case http.MethodPost:
			s.createRecord(w, r, user, subID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 1 && parts[0] == "sync" && r.Method == http.MethodPost {
		s.syncRecords(w, r, user, subID)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.deleteRecord(w, r, user, subID, parts[0])
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPatch {
		s.updateRecord(w, r, user, subID, parts[0])
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) deleteRecord(w http.ResponseWriter, r *http.Request, user User, subID, recordID string) {
	var cfRecordID, zoneID, encryptedToken string
	err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(dr.cloudflare_record_id, ''), d.zone_id, d.api_token_encrypted
		FROM dns_records dr JOIN subdomains s ON s.id = dr.subdomain_id JOIN domains d ON d.id = s.domain_id
		WHERE dr.id = ? AND dr.subdomain_id = ? AND (s.owner_id = ? OR ? = 'admin')`, recordID, subID, user.ID, user.Role).Scan(&cfRecordID, &zoneID, &encryptedToken)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load record")
		return
	}
	if cfRecordID != "" {
		token, err := s.decrypt(encryptedToken)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not decrypt Cloudflare token")
			return
		}
		if err := s.cloudflareDeleteRecord(r.Context(), zoneID, token, cfRecordID); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE dr FROM dns_records dr JOIN subdomains s ON s.id = dr.subdomain_id WHERE dr.id = ? AND dr.subdomain_id = ? AND (s.owner_id = ? OR ? = 'admin')`, recordID, subID, user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete record")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRecords(w http.ResponseWriter, r *http.Request, user User, subID string) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT dr.id, dr.subdomain_id, COALESCE(dr.cloudflare_record_id, ''), dr.type, dr.name, dr.content, dr.ttl, dr.proxied, dr.created_at
		FROM dns_records dr JOIN subdomains s ON s.id = dr.subdomain_id
		WHERE dr.subdomain_id = ? AND (s.owner_id = ? OR ? = 'admin')
		ORDER BY dr.created_at DESC`, subID, user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list records")
		return
	}
	defer rows.Close()
	records, err := scanRecords(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not scan records")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request, user User, subID string) {
	var req struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Proxied bool   `json:"proxied"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	recordType := strings.ToUpper(strings.TrimSpace(req.Type))
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "@"
	}
	content := strings.TrimSpace(req.Content)
	if !slices.Contains(recordTypes, recordType) || !namePattern.MatchString(name) || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, "invalid DNS record name or type")
		return
	}
	if err := validateDNSRecordContent(recordType, content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TTL == 0 {
		req.TTL = 1
	}
	if !slices.Contains(ttls, req.TTL) {
		writeError(w, http.StatusBadRequest, "invalid ttl")
		return
	}
	if req.Proxied && !slices.Contains([]string{"A", "AAAA", "CNAME"}, recordType) {
		writeError(w, http.StatusBadRequest, "proxied records must be A, AAAA, or CNAME")
		return
	}
	var status, prefix, domainName, zoneID, encryptedToken string
	err := s.db.QueryRowContext(r.Context(), `SELECT s.status, s.prefix, d.name, d.zone_id, d.api_token_encrypted
		FROM subdomains s JOIN domains d ON d.id = s.domain_id
		WHERE s.id = ? AND (s.owner_id = ? OR ? = 'admin')`, subID, user.ID, user.Role).Scan(&status, &prefix, &domainName, &zoneID, &encryptedToken)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load subdomain")
		return
	}
	if status != "active" {
		writeError(w, http.StatusBadRequest, "DNS records can only be changed for active subdomains")
		return
	}
	token, err := s.decrypt(encryptedToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not decrypt Cloudflare token")
		return
	}
	fullName := recordName(name, prefix, domainName)
	cfRecordID, err := s.cloudflareCreateRecord(r.Context(), zoneID, token, DNSRecord{Type: recordType, Name: fullName, Content: content, TTL: req.TTL, Proxied: req.Proxied})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	id := newID("dns")
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO dns_records (id, subdomain_id, type, name, content, ttl, proxied, cloudflare_record_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, subID, recordType, name, content, req.TTL, req.Proxied, cfRecordID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create record")
		return
	}
	writeJSON(w, http.StatusCreated, DNSRecord{ID: id, SubdomainID: subID, CloudflareRecordID: cfRecordID, Type: recordType, Name: name, Content: content, TTL: req.TTL, Proxied: req.Proxied, CreatedAt: s.now()})
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request, user User, subID, recordID string) {
	var req struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Proxied bool   `json:"proxied"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	recordType := strings.ToUpper(strings.TrimSpace(req.Type))
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "@"
	}
	content := strings.TrimSpace(req.Content)
	if !slices.Contains(recordTypes, recordType) || !namePattern.MatchString(name) || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, "invalid DNS record name or type")
		return
	}
	if err := validateDNSRecordContent(recordType, content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TTL == 0 {
		req.TTL = 1
	}
	if !slices.Contains(ttls, req.TTL) {
		writeError(w, http.StatusBadRequest, "invalid ttl")
		return
	}
	if req.Proxied && !slices.Contains([]string{"A", "AAAA", "CNAME"}, recordType) {
		writeError(w, http.StatusBadRequest, "proxied records must be A, AAAA, or CNAME")
		return
	}

	var status, prefix, domainName, zoneID, encryptedToken, cfRecordID string
	var createdAt time.Time
	err := s.db.QueryRowContext(r.Context(), `SELECT s.status, s.prefix, d.name, d.zone_id, d.api_token_encrypted, COALESCE(dr.cloudflare_record_id, ''), dr.created_at
		FROM dns_records dr JOIN subdomains s ON s.id = dr.subdomain_id JOIN domains d ON d.id = s.domain_id
		WHERE dr.id = ? AND dr.subdomain_id = ? AND (s.owner_id = ? OR ? = 'admin')`, recordID, subID, user.ID, user.Role).
		Scan(&status, &prefix, &domainName, &zoneID, &encryptedToken, &cfRecordID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load record")
		return
	}
	if status != "active" {
		writeError(w, http.StatusBadRequest, "DNS records can only be changed for active subdomains")
		return
	}
	if cfRecordID == "" {
		writeError(w, http.StatusBadRequest, "record is not linked to a Cloudflare DNS record")
		return
	}
	token, err := s.decrypt(encryptedToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not decrypt Cloudflare token")
		return
	}
	fullName := recordName(name, prefix, domainName)
	if err := s.cloudflareUpdateRecord(r.Context(), zoneID, token, cfRecordID, DNSRecord{Type: recordType, Name: fullName, Content: content, TTL: req.TTL, Proxied: req.Proxied}); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE dns_records SET type = ?, name = ?, content = ?, ttl = ?, proxied = ? WHERE id = ? AND subdomain_id = ?`, recordType, name, content, req.TTL, req.Proxied, recordID, subID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update record")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	writeJSON(w, http.StatusOK, DNSRecord{ID: recordID, SubdomainID: subID, CloudflareRecordID: cfRecordID, Type: recordType, Name: name, Content: content, TTL: req.TTL, Proxied: req.Proxied, CreatedAt: createdAt})
}

func (s *Server) syncRecords(w http.ResponseWriter, r *http.Request, user User, subID string) {
	var status, prefix, domainName, zoneID, encryptedToken string
	err := s.db.QueryRowContext(r.Context(), `SELECT s.status, s.prefix, d.name, d.zone_id, d.api_token_encrypted
		FROM subdomains s JOIN domains d ON d.id = s.domain_id
		WHERE s.id = ? AND (s.owner_id = ? OR ? = 'admin')`, subID, user.ID, user.Role).Scan(&status, &prefix, &domainName, &zoneID, &encryptedToken)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "subdomain not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load subdomain")
		return
	}
	if status != "active" {
		writeError(w, http.StatusBadRequest, "DNS records can only be synced for active subdomains")
		return
	}
	token, err := s.decrypt(encryptedToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not decrypt Cloudflare token")
		return
	}
	fullDomain := prefix + "." + domainName
	cloudflareRecords, err := s.cloudflareListRecords(r.Context(), zoneID, token, fullDomain)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start sync")
		return
	}
	defer tx.Rollback()

	seen := make([]string, 0, len(cloudflareRecords))
	for _, record := range cloudflareRecords {
		localName, ok := localRecordName(record.Name, fullDomain)
		if !ok || !slices.Contains(recordTypes, record.Type) {
			continue
		}
		if record.TTL == 0 {
			record.TTL = 1
		}
		seen = append(seen, record.ID)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO dns_records (id, subdomain_id, type, name, content, ttl, proxied, cloudflare_record_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE id = id, type = VALUES(type), name = VALUES(name), content = VALUES(content), ttl = VALUES(ttl), proxied = VALUES(proxied)`,
			newID("dns"), subID, record.Type, localName, record.Content, record.TTL, record.Proxied, record.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not upsert synced record")
			return
		}
	}
	if len(seen) == 0 {
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dns_records WHERE subdomain_id = ? AND cloudflare_record_id IS NOT NULL AND cloudflare_record_id <> ''`, subID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not prune synced records")
			return
		}
	} else {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(seen)), ",")
		args := make([]any, 0, len(seen)+1)
		args = append(args, subID)
		for _, id := range seen {
			args = append(args, id)
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM dns_records WHERE subdomain_id = ? AND cloudflare_record_id IS NOT NULL AND cloudflare_record_id <> '' AND cloudflare_record_id NOT IN (`+placeholders+`)`, args...); err != nil {
			writeError(w, http.StatusInternalServerError, "could not prune synced records")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not finish sync")
		return
	}
	s.listRecords(w, r, user, subID)
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request, parts []string) {
	user := currentUser(r)
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			rows, err := s.db.QueryContext(r.Context(), `SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`, user.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not list tokens")
				return
			}
			defer rows.Close()
			tokens, err := scanTokens(rows)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not scan tokens")
				return
			}
			writeJSON(w, http.StatusOK, tokens)
		case http.MethodPost:
			var req struct {
				Name string `json:"name"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			name := strings.TrimSpace(req.Name)
			if name == "" {
				writeError(w, http.StatusBadRequest, "token name is required")
				return
			}
			secret, err := newToken()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not generate token")
				return
			}
			token := APIToken{ID: newID("tok"), UserID: user.ID, Name: name, TokenHash: hashToken(secret), CreatedAt: s.now()}
			_, err = s.db.ExecContext(r.Context(), `INSERT INTO api_tokens (id, user_id, name, token_hash) VALUES (?, ?, ?, ?)`, token.ID, token.UserID, token.Name, token.TokenHash)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not create token")
				return
			}
			writeJSON(w, http.StatusCreated, tokenResponse{APIToken: token, Token: secret})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		res, err := s.db.ExecContext(r.Context(), `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, parts[0], user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not delete token")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "rotate" && r.Method == http.MethodPost {
		var token APIToken
		err := s.db.QueryRowContext(r.Context(), `SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE id = ? AND user_id = ?`, parts[0], user.ID).
			Scan(&token.ID, &token.UserID, &token.Name, &token.TokenHash, &token.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load token")
			return
		}
		secret, err := newToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not generate token")
			return
		}
		_, err = s.db.ExecContext(r.Context(), `UPDATE api_tokens SET token_hash = ?, created_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`, hashToken(secret), token.ID, user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not rotate token")
			return
		}
		token.TokenHash = hashToken(secret)
		token.CreatedAt = s.now()
		writeJSON(w, http.StatusOK, tokenResponse{APIToken: token, Token: secret})
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request, parts []string) {
	user := currentUser(r)
	if user.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}
	if len(parts) == 0 {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	switch parts[0] {
	case "domains":
		s.handleAdminDomains(w, r, parts[1:])
	case "subdomains":
		s.handleAdminSubdomains(w, r, parts[1:])
	case "users":
		s.handleAdminUsers(w, r, parts[1:])
	default:
		writeError(w, http.StatusNotFound, "route not found")
	}
}

func (s *Server) handleAdminDomains(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			rows, err := s.db.QueryContext(r.Context(), `SELECT id, name, zone_id, enabled, created_at, updated_at FROM domains ORDER BY name`)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not list domains")
				return
			}
			defer rows.Close()
			domains, err := scanDomains(rows)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not scan domains")
				return
			}
			writeJSON(w, http.StatusOK, domains)
		case http.MethodPost:
			var req struct {
				Name     string `json:"name"`
				ZoneID   string `json:"zoneId"`
				APIToken string `json:"apiToken"`
				Enabled  bool   `json:"enabled"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			name := strings.ToLower(strings.Trim(strings.TrimSpace(req.Name), "."))
			if !domainPattern.MatchString(name) || strings.TrimSpace(req.ZoneID) == "" || strings.TrimSpace(req.APIToken) == "" {
				writeError(w, http.StatusBadRequest, "domain name, zone id, and api token are required")
				return
			}
			encrypted, err := s.encrypt(req.APIToken)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not encrypt token")
				return
			}
			id := newID("dom")
			_, err = s.db.ExecContext(r.Context(), `INSERT INTO domains (id, name, zone_id, api_token_encrypted, enabled) VALUES (?, ?, ?, ?, ?)`, id, name, req.ZoneID, encrypted, req.Enabled)
			if isDuplicate(err) {
				writeError(w, http.StatusConflict, "domain already exists")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not create domain")
				return
			}
			writeJSON(w, http.StatusCreated, Domain{ID: id, Name: name, ZoneID: req.ZoneID, Enabled: req.Enabled, TokenMasked: "********", CreatedAt: s.now(), UpdatedAt: s.now()})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				Name     *string `json:"name"`
				ZoneID   *string `json:"zoneId"`
				APIToken *string `json:"apiToken"`
				Enabled  *bool   `json:"enabled"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			if req.Name != nil {
				name := strings.ToLower(strings.Trim(strings.TrimSpace(*req.Name), "."))
				if !domainPattern.MatchString(name) {
					writeError(w, http.StatusBadRequest, "invalid domain name")
					return
				}
				if _, err := s.db.ExecContext(r.Context(), `UPDATE domains SET name = ? WHERE id = ?`, name, parts[0]); err != nil {
					writeError(w, http.StatusInternalServerError, "could not update domain")
					return
				}
			}
			if req.ZoneID != nil {
				if _, err := s.db.ExecContext(r.Context(), `UPDATE domains SET zone_id = ? WHERE id = ?`, *req.ZoneID, parts[0]); err != nil {
					writeError(w, http.StatusInternalServerError, "could not update zone id")
					return
				}
			}
			if req.APIToken != nil && strings.TrimSpace(*req.APIToken) != "" {
				encrypted, err := s.encrypt(*req.APIToken)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not encrypt token")
					return
				}
				if _, err := s.db.ExecContext(r.Context(), `UPDATE domains SET api_token_encrypted = ? WHERE id = ?`, encrypted, parts[0]); err != nil {
					writeError(w, http.StatusInternalServerError, "could not update api token")
					return
				}
			}
			if req.Enabled != nil {
				if _, err := s.db.ExecContext(r.Context(), `UPDATE domains SET enabled = ? WHERE id = ?`, *req.Enabled, parts[0]); err != nil {
					writeError(w, http.StatusInternalServerError, "could not update domain")
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		case http.MethodDelete:
			res, err := s.db.ExecContext(r.Context(), `DELETE FROM domains WHERE id = ?`, parts[0])
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not delete domain")
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				writeError(w, http.StatusNotFound, "domain not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) handleAdminSubdomains(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 && r.Method == http.MethodGet {
		s.listSubdomains(w, r, currentUser(r), true)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "approve":
			s.reviewSubdomain(w, r, parts[0], "active")
			return
		case "reject":
			s.reviewSubdomain(w, r, parts[0], "rejected")
			return
		}
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) reviewSubdomain(w http.ResponseWriter, r *http.Request, id, status string) {
	var req struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	user := currentUser(r)
	_, err := s.db.ExecContext(r.Context(), `UPDATE subdomains SET status = ?, reject_reason = ?, reviewed_by = ?, reviewed_at = ? WHERE id = ?`, status, nullable(req.Reason), user.ID, s.now(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not review subdomain")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 && r.Method == http.MethodGet {
		rows, err := s.db.QueryContext(r.Context(), `SELECT id, email, nickname, role, created_at FROM users ORDER BY created_at DESC`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list users")
			return
		}
		defer rows.Close()
		users, err := scanUsers(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not scan users")
			return
		}
		writeJSON(w, http.StatusOK, users)
		return
	}
	if len(parts) == 0 && r.Method == http.MethodPost {
		var req struct {
			Email    string `json:"email"`
			Nickname string `json:"nickname"`
			Role     string `json:"role"`
			Password string `json:"password"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		nickname := strings.TrimSpace(req.Nickname)
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if email == "" || !strings.Contains(email, "@") {
			writeError(w, http.StatusBadRequest, "valid email is required")
			return
		}
		if nickname == "" {
			nickname = email
		}
		if role == "" {
			role = "user"
		}
		if role != "user" && role != "admin" {
			writeError(w, http.StatusBadRequest, "role must be user or admin")
			return
		}
		if len(req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not hash password")
			return
		}
		user := User{ID: newID("usr"), Email: email, Nickname: nickname, Role: role, CreatedAt: s.now()}
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO users (id, email, nickname, role, password_hash) VALUES (?, ?, ?, ?, ?)`, user.ID, user.Email, user.Nickname, user.Role, string(hash))
		if isDuplicate(err) {
			writeError(w, http.StatusConflict, "email already exists")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create user")
			return
		}
		writeJSON(w, http.StatusCreated, user)
		return
	}
	if len(parts) == 2 && parts[1] == "reset-password" && r.Method == http.MethodPost {
		var req struct {
			NewPassword string `json:"newPassword"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if len(req.NewPassword) < 8 {
			writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not hash password")
			return
		}
		res, err := s.db.ExecContext(r.Context(), `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), parts[0])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not reset password")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) dnsQuery(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimSpace(r.URL.Query().Get("name")), ".")
	recordType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	if !domainPattern.MatchString(strings.ToLower(name)) || !slices.Contains(recordTypes, recordType) {
		writeError(w, http.StatusBadRequest, "invalid DNS query")
		return
	}
	endpoint := "https://cloudflare-dns.com/dns-query?name=" + url.QueryEscape(name) + "&type=" + url.QueryEscape(recordType)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create DNS request")
		return
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := s.http.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "DNS query failed")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read DNS response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (s *Server) userByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, email, nickname, role, password_hash, created_at FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email))).Scan(&u.ID, &u.Email, &u.Nickname, &u.Role, &hash, &u.CreatedAt)
	return u, hash, err
}

func (s *Server) userByID(ctx context.Context, id string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id, email, nickname, role, created_at FROM users WHERE id = ?`, id).Scan(&u.ID, &u.Email, &u.Nickname, &u.Role, &u.CreatedAt)
	return u, err
}

func (s *Server) userByAPIToken(ctx context.Context, token string) (User, bool) {
	var userID string
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM api_tokens WHERE token_hash = ?`, hashToken(token)).Scan(&userID); err != nil {
		return User{}, false
	}
	user, err := s.userByID(ctx, userID)
	return user, err == nil
}

func (s *Server) issueJWT(user User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"role":  user.Role,
		"exp":   s.now().Add(24 * time.Hour).Unix(),
		"iat":   s.now().Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *Server) userFromJWT(ctx context.Context, raw string) (User, error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return User{}, fmt.Errorf("invalid token")
	}
	sub, err := token.Claims.GetSubject()
	if err != nil || sub == "" {
		return User{}, fmt.Errorf("missing subject")
	}
	return s.userByID(ctx, sub)
}

func (s *Server) encrypt(value string) (string, error) {
	block, err := aes.NewCipher(s.tokenKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	return base64.RawURLEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *Server) decrypt(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.tokenKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted value is too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Server) ensureColumn(ctx context.Context, table, column, alter string) error {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, alter)
	return err
}

func (s *Server) ensureIndex(ctx context.Context, table, index, alter string) error {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`, table, index).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, alter)
	return err
}

func (s *Server) cloudflareCreateRecord(ctx context.Context, zoneID, token string, record DNSRecord) (string, error) {
	body := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}
	var response struct {
		Success bool `json:"success"`
		Result  struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := s.cloudflareRequest(ctx, http.MethodPost, zoneID, token, "/dns_records", body, &response); err != nil {
		return "", err
	}
	if !response.Success || response.Result.ID == "" {
		return "", fmt.Errorf("Cloudflare create record failed: %s", cloudflareError(response.Errors))
	}
	return response.Result.ID, nil
}

func (s *Server) cloudflareUpdateRecord(ctx context.Context, zoneID, token, recordID string, record DNSRecord) error {
	body := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Content,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}
	var response struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := s.cloudflareRequest(ctx, http.MethodPatch, zoneID, token, "/dns_records/"+url.PathEscape(recordID), body, &response); err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("Cloudflare update record failed: %s", cloudflareError(response.Errors))
	}
	return nil
}

func (s *Server) cloudflareDeleteRecord(ctx context.Context, zoneID, token, recordID string) error {
	var response struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := s.cloudflareRequest(ctx, http.MethodDelete, zoneID, token, "/dns_records/"+url.PathEscape(recordID), nil, &response); err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("Cloudflare delete record failed: %s", cloudflareError(response.Errors))
	}
	return nil
}

type cloudflareDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func (s *Server) cloudflareListRecords(ctx context.Context, zoneID, token, fullDomain string) ([]cloudflareDNSRecord, error) {
	var all []cloudflareDNSRecord
	for page := 1; ; page++ {
		var response struct {
			Success bool                  `json:"success"`
			Result  []cloudflareDNSRecord `json:"result"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
			ResultInfo struct {
				Page       int `json:"page"`
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
		}
		path := fmt.Sprintf("/dns_records?per_page=100&page=%d&name.endswith=%s", page, url.QueryEscape(fullDomain))
		if err := s.cloudflareRequest(ctx, http.MethodGet, zoneID, token, path, nil, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, fmt.Errorf("Cloudflare list records failed: %s", cloudflareError(response.Errors))
		}
		for _, record := range response.Result {
			if _, ok := localRecordName(record.Name, fullDomain); ok {
				all = append(all, record)
			}
		}
		if response.ResultInfo.TotalPages == 0 || page >= response.ResultInfo.TotalPages {
			break
		}
	}
	return all, nil
}

func (s *Server) cloudflareRequest(ctx context.Context, method, zoneID, token, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4/zones/"+url.PathEscape(zoneID)+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if contentType != "" && contentType != "application/json" {
		return fmt.Errorf("Cloudflare returned %s", contentType)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("could not decode Cloudflare response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("Cloudflare returned HTTP 404. The DNS record may have been deleted directly in Cloudflare; refresh or remove the local record after verifying")
		}
		return fmt.Errorf("Cloudflare returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func cloudflareError(errors []struct {
	Message string `json:"message"`
}) string {
	if len(errors) == 0 {
		return "unknown error"
	}
	messages := make([]string, 0, len(errors))
	for _, item := range errors {
		messages = append(messages, item.Message)
	}
	return strings.Join(messages, "; ")
}

func recordName(name, prefix, domainName string) string {
	if name == "@" {
		return prefix + "." + domainName
	}
	return name + "." + prefix + "." + domainName
}

func validateDNSRecordContent(recordType, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content is required")
	}
	switch recordType {
	case "A":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("A record content must be a valid IPv4 address")
		}
	case "AAAA":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil || ip.To16() == nil {
			return fmt.Errorf("AAAA record content must be a valid IPv6 address")
		}
	case "CNAME", "NS":
		if !domainPattern.MatchString(strings.ToLower(strings.Trim(content, "."))) {
			return fmt.Errorf("%s record content must be a valid domain name", recordType)
		}
	case "MX":
		parts := strings.Fields(content)
		if len(parts) == 2 {
			if _, err := strconv.Atoi(parts[0]); err != nil {
				return fmt.Errorf("MX record content must be a domain name or '<priority> <domain>'")
			}
			content = parts[1]
		}
		if !domainPattern.MatchString(strings.ToLower(strings.Trim(content, "."))) {
			return fmt.Errorf("MX record content must be a domain name or '<priority> <domain>'")
		}
	case "TXT":
		if len(content) > 4096 {
			return fmt.Errorf("TXT record content is too long")
		}
	}
	return nil
}

func localRecordName(name, fullDomain string) (string, bool) {
	name = strings.Trim(strings.ToLower(name), ".")
	fullDomain = strings.Trim(strings.ToLower(fullDomain), ".")
	if name == fullDomain {
		return "@", true
	}
	suffix := "." + fullDomain
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	local := strings.TrimSuffix(name, suffix)
	if local == "" || strings.Contains(local, "..") {
		return "", false
	}
	return local, true
}

func scanUsers(rows *sql.Rows) ([]User, error) {
	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func scanDomains(rows *sql.Rows) ([]Domain, error) {
	out := make([]Domain, 0)
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.Name, &d.ZoneID, &d.Enabled, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.TokenMasked = "********"
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanSubdomains(rows *sql.Rows) ([]Subdomain, error) {
	out := make([]Subdomain, 0)
	for rows.Next() {
		var s Subdomain
		var reviewedAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.OwnerEmail, &s.DomainID, &s.DomainName, &s.Prefix, &s.Status, &s.RejectReason, &s.ReviewedBy, &reviewedAt, &s.CreatedAt); err != nil {
			return nil, err
		}
		if reviewedAt.Valid {
			s.ReviewedAt = &reviewedAt.Time
		}
		s.FullDomain = s.Prefix + "." + s.DomainName
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanRecords(rows *sql.Rows) ([]DNSRecord, error) {
	out := make([]DNSRecord, 0)
	for rows.Next() {
		var r DNSRecord
		if err := rows.Scan(&r.ID, &r.SubdomainID, &r.CloudflareRecordID, &r.Type, &r.Name, &r.Content, &r.TTL, &r.Proxied, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanTokens(rows *sql.Rows) ([]APIToken, error) {
	out := make([]APIToken, 0)
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func currentUser(r *http.Request) User {
	user, _ := r.Context().Value(userContextKey).(User)
	return user
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func splitPath(path string) []string {
	var parts []string
	for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func bearerToken(header string) string {
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tkn_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func isDuplicate(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

func ensureDSNOptions(dsn string) string {
	if strings.Contains(dsn, "?") {
		if !strings.Contains(dsn, "parseTime=") {
			return dsn + "&parseTime=true"
		}
		return dsn
	}
	return dsn + "?parseTime=true"
}

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// BufferedResponseWriter is used by tests that need to inspect JSON without a network socket.
type BufferedResponseWriter struct {
	HeaderMap http.Header
	Body      bytes.Buffer
	Status    int
}

func (w *BufferedResponseWriter) Header() http.Header {
	if w.HeaderMap == nil {
		w.HeaderMap = http.Header{}
	}
	return w.HeaderMap
}

func (w *BufferedResponseWriter) WriteHeader(statusCode int)  { w.Status = statusCode }
func (w *BufferedResponseWriter) Write(b []byte) (int, error) { return w.Body.Write(b) }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
