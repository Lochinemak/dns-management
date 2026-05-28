package main

import (
	"context"
	"encoding/json"
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
