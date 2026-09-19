package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/instance"
	"github.com/gogu-x/ops/ops/internal/application"
	"github.com/gogu-x/ops/ops/internal/project"
	"github.com/gogu-x/ops/ops/internal/service"
	"github.com/gogu-x/ops/ops/internal/store"
)

func newTestActor(t *testing.T) *Actor {
	t.Helper()
	repo := store.NewMemoryRepository()
	app := application.New(
		application.NewTreeGateway(20*time.Second),
		project.NewMemoryRepository(),
		service.NewMemoryRepository(),
		instance.NewMemoryRepository(),
	)
	a := NewActor(conf.Config{
		JWTSecret:      "test-secret",
		AccessTTLMin:   15,
		RefreshTTLDays: 7,
		AdminPassword:  "admin-password",
	}, repo, app)
	if a == nil {
		t.Fatal("NewActor returned nil")
	}
	if err := a.auth.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap auth: %v", err)
	}
	return a
}

func TestRouterHealthAndAuthentication(t *testing.T) {
	a := newTestActor(t)
	router := a.newRouter()

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/me status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	loginBody := bytes.NewBufferString(`{"username":"admin","password":"admin-password"}`)
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRequest.Header.Set("Content-Type", "application/json")
	login := httptest.NewRecorder()
	router.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body=%s", login.Code, http.StatusOK, login.Body.String())
	}

	var response struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.Data.AccessToken == "" {
		t.Fatal("login response has no access token")
	}

	pingRequest := httptest.NewRequest(http.MethodGet, "/api/admin/ping", nil)
	pingRequest.Header.Set("Authorization", "Bearer "+response.Data.AccessToken)
	ping := httptest.NewRecorder()
	router.ServeHTTP(ping, pingRequest)
	if ping.Code != http.StatusOK {
		t.Fatalf("admin ping status = %d, want %d; body=%s", ping.Code, http.StatusOK, ping.Body.String())
	}
}

func TestRouterCORSPreflight(t *testing.T) {
	router := newTestActor(t).newRouter()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodOptions, "/api/projects", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("preflight response has no Access-Control-Allow-Methods header")
	}
}
