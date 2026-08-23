package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginSuccessAndJWTMe(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	srv.now = func() time.Time { return mustTime("2026-05-28T12:00:00Z") }

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	userRows := sqlmock.NewRows([]string{"id", "email", "nickname", "role", "password_hash", "created_at"}).
		AddRow("u-test", "user@example.com", "User", "user", string(hash), mustTime("2026-05-28T10:00:00Z"))
	mock.ExpectQuery("SELECT id, email, nickname, role, password_hash, created_at FROM users WHERE email = ?").
		WithArgs("user@example.com").
		WillReturnRows(userRows)

	resp := request(srv, http.MethodPost, "/api/v1/auth/login", `{"email":"user@example.com","password":"password123"}`, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", resp.Code, resp.Body.String())
	}
	var login struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.Token == "" || login.User.ID != "u-test" {
		t.Fatalf("unexpected login response: %+v", login)
	}

	meRows := sqlmock.NewRows([]string{"id", "email", "nickname", "role", "created_at"}).
		AddRow("u-test", "user@example.com", "User", "user", mustTime("2026-05-28T10:00:00Z"))
	mock.ExpectQuery("SELECT id, email, nickname, role, created_at FROM users WHERE id = ?").
		WithArgs("u-test").
		WillReturnRows(meRows)

	resp = request(srv, http.MethodGet, "/api/v1/me", "", login.Token)
	if resp.Code != http.StatusOK {
		t.Fatalf("me status = %d body = %s", resp.Code, resp.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminEndpointRejectsNormalUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	user := User{ID: "u-test", Email: "user@example.com", Nickname: "User", Role: "user"}
	token, err := srv.issueJWT(user)
	if err != nil {
		t.Fatal(err)
	}

	rows := sqlmock.NewRows([]string{"id", "email", "nickname", "role", "created_at"}).
		AddRow("u-test", "user@example.com", "User", "user", mustTime("2026-05-28T10:00:00Z"))
	mock.ExpectQuery("SELECT id, email, nickname, role, created_at FROM users WHERE id = ?").
		WithArgs("u-test").
		WillReturnRows(rows)

	resp := request(srv, http.MethodGet, "/api/v1/admin/domains", "", token)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("admin status = %d body = %s", resp.Code, resp.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDNSQueryValidation(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	user := User{ID: "u-test", Email: "user@example.com", Nickname: "User", Role: "user"}
	req := contextWithUser(httptest.NewRequest(http.MethodGet, "/api/v1/dns-query?name=-bad&type=A", nil), user)
	rec := httptest.NewRecorder()
	srv.dnsQuery(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dns validation status = %d", rec.Code)
	}
}

func TestCreateSubdomainRejectsReservedPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	user := User{ID: "u-test", Email: "user@example.com", Nickname: "User", Role: "user"}

	mock.ExpectQuery("SELECT name FROM domains WHERE id = \\? AND enabled = TRUE").
		WithArgs("dom-1").
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("example.com"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM reserved_subdomains WHERE domain_id = \\? AND prefix = \\?").
		WithArgs("dom-1", "admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := contextWithUser(httptest.NewRequest(http.MethodPost, "/api/v1/subdomains", strings.NewReader(`{"domainId":"dom-1","prefix":"admin"}`)), user)
	rec := httptest.NewRecorder()
	srv.createSubdomain(rec, req, user)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reserved status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "subdomain is reserved") {
		t.Fatalf("reserved body = %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSubdomainAllowsSamePrefixOnDifferentDomain(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	srv.now = func() time.Time { return mustTime("2026-05-28T12:00:00Z") }
	user := User{ID: "u-test", Email: "user@example.com", Nickname: "User", Role: "user"}

	mock.ExpectQuery("SELECT name FROM domains WHERE id = \\? AND enabled = TRUE").
		WithArgs("dom-2").
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("example.net"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM reserved_subdomains WHERE domain_id = \\? AND prefix = \\?").
		WithArgs("dom-2", "admin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO subdomains").
		WithArgs(sqlmock.AnyArg(), "u-test", "dom-2", "admin").
		WillReturnResult(sqlmock.NewResult(1, 1))

	req := contextWithUser(httptest.NewRequest(http.MethodPost, "/api/v1/subdomains", strings.NewReader(`{"domainId":"dom-2","prefix":"admin"}`)), user)
	rec := httptest.NewRecorder()
	srv.createSubdomain(rec, req, user)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReservedSubdomainEndpointRejectsNormalUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	user := User{ID: "u-test", Email: "user@example.com", Nickname: "User", Role: "user"}
	token, err := srv.issueJWT(user)
	if err != nil {
		t.Fatal(err)
	}

	rows := sqlmock.NewRows([]string{"id", "email", "nickname", "role", "created_at"}).
		AddRow("u-test", "user@example.com", "User", "user", mustTime("2026-05-28T10:00:00Z"))
	mock.ExpectQuery("SELECT id, email, nickname, role, created_at FROM users WHERE id = ?").
		WithArgs("u-test").
		WillReturnRows(rows)

	resp := request(srv, http.MethodGet, "/api/v1/admin/reserved-subdomains", "", token)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("reserved admin status = %d body = %s", resp.Code, resp.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReservedSubdomainRejectsInvalidPrefix(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	admin := User{ID: "u-admin", Email: "admin@example.com", Nickname: "Admin", Role: "admin"}

	req := contextWithUser(httptest.NewRequest(http.MethodPost, "/api/v1/admin/reserved-subdomains", strings.NewReader(`{"domainId":"dom-1","prefix":"-bad"}`)), admin)
	rec := httptest.NewRecorder()
	srv.handleAdminReservedSubdomains(rec, req, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid reserved status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateReservedSubdomainRejectsDuplicateRule(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := NewServer(db, "test-secret", "test-secret")
	admin := User{ID: "u-admin", Email: "admin@example.com", Nickname: "Admin", Role: "admin"}

	mock.ExpectQuery("SELECT name FROM domains WHERE id = \\?").
		WithArgs("dom-1").
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("example.com"))
	mock.ExpectExec("INSERT INTO reserved_subdomains").
		WithArgs(sqlmock.AnyArg(), "dom-1", "admin", "u-admin").
		WillReturnError(errors.New("constraint failed: UNIQUE constraint failed: reserved_subdomains.domain_id, reserved_subdomains.prefix (2067)"))

	req := contextWithUser(httptest.NewRequest(http.MethodPost, "/api/v1/admin/reserved-subdomains", strings.NewReader(`{"domainId":"dom-1","prefix":"admin"}`)), admin)
	rec := httptest.NewRecorder()
	srv.handleAdminReservedSubdomains(rec, req, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate reserved status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteMigrationAndSetup(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "dns-management.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{`PRAGMA foreign_keys = ON`, `PRAGMA journal_mode = WAL`, `PRAGMA busy_timeout = 5000`} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer(db, "test-secret", "test-token-secret")
	if err := srv.migrate(context.Background()); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := srv.migrate(context.Background()); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}

	status := request(srv, http.MethodGet, "/api/v1/setup/status", "", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"initialized":false`) {
		t.Fatalf("initial setup status = %d body = %s", status.Code, status.Body.String())
	}

	created := request(srv, http.MethodPost, "/api/v1/setup/admin", `{"email":"admin@example.com","nickname":"Admin","password":"password123"}`, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("create admin status = %d body = %s", created.Code, created.Body.String())
	}

	status = request(srv, http.MethodGet, "/api/v1/setup/status", "", "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"initialized":true`) {
		t.Fatalf("initialized setup status = %d body = %s", status.Code, status.Body.String())
	}
}

func TestSQLiteDNSRecordUpsert(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dns-management.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(db, "test-secret", "test-token-secret")
	if err := srv.migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, nickname, role, password_hash) VALUES ('usr-1', 'user@example.com', 'User', 'user', 'hash')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO domains (id, name, zone_id, api_token_encrypted) VALUES ('dom-1', 'example.com', 'zone-1', 'token')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subdomains (id, owner_id, domain_id, prefix, status) VALUES ('sub-1', 'usr-1', 'dom-1', 'www', 'active')`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(upsertDNSRecordSQL, "dns-1", "sub-1", "A", "@", "192.0.2.1", 60, false, "cf-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(upsertDNSRecordSQL, "dns-2", "sub-1", "A", "@", "192.0.2.2", 3600, true, "cf-1"); err != nil {
		t.Fatal(err)
	}

	var id, content string
	var ttl, count int
	var proxied bool
	if err := db.QueryRow(`SELECT id, content, ttl, proxied FROM dns_records WHERE cloudflare_record_id = 'cf-1'`).Scan(&id, &content, &ttl, &proxied); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM dns_records WHERE cloudflare_record_id = 'cf-1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if id != "dns-1" || content != "192.0.2.2" || ttl != 3600 || !proxied || count != 1 {
		t.Fatalf("upsert result: id=%q content=%q ttl=%d proxied=%v count=%d", id, content, ttl, proxied, count)
	}
}

func TestFrontendServesExportedCleanURLHTML(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("home page"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "admin.html"), []byte("admin page"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(db, "test-secret", "test-secret")
	srv.frontendDir = dir

	resp := request(srv, http.MethodGet, "/admin", "", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("frontend status = %d body = %s", resp.Code, resp.Body.String())
	}
	if body := resp.Body.String(); body != "admin page" {
		t.Fatalf("frontend body = %q, want admin page", body)
	}
}

func request(srv *Server, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	return rec
}

func contextWithUser(req *http.Request, user User) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), userContextKey, user))
}

func mustTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}
