package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/admin/internal/auth"
	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/admin/internal/store"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/ops/model"
)

func newTestActor(t *testing.T) *AdminService {
	t.Helper()
	conf.JWTSecret = "test-secret"
	conf.AccessTTLMin = 15
	conf.RefreshTTLDays = 7
	conf.AdminPassword = "admin-password"

	repo := store.NewMemoryRepository()
	app := application.New(
		application.NewTreeGateway(20*time.Second),
		project.NewMemoryRepository(),
		environment.NewMemoryRepository(),
		service.NewMemoryRepository(),
		instance.NewMemoryRepository(),
	)
	a := &AdminService{auth: auth.NewService(repo), app: app}
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

func TestManagedUserProjectScopeAndPermissions(t *testing.T) {
	a := newTestActor(t)
	router := a.newRouter()
	ctx := context.Background()
	allowedProject, err := a.app.CreateProject(ctx, model.Project{Name: "allowed"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.app.CreateProject(ctx, model.Project{Name: "private"})
	if err != nil {
		t.Fatal(err)
	}

	adminToken := loginToken(t, router, "admin", "admin-password")
	create := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(`{"username":"viewer1","password":"viewer-password","role":"user","project_ids":["`+allowedProject.ID+`"],"permissions":["projects.view"]}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Authorization", "Bearer "+adminToken)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, want %d; body=%s", created.Code, http.StatusCreated, created.Body.String())
	}
	var createdUser struct {
		Data model.User `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdUser); err != nil {
		t.Fatal(err)
	}

	userToken := loginToken(t, router, "viewer1", "viewer-password")
	projectsRequest := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	projectsRequest.Header.Set("Authorization", "Bearer "+userToken)
	projectsResponse := httptest.NewRecorder()
	router.ServeHTTP(projectsResponse, projectsRequest)
	if projectsResponse.Code != http.StatusOK {
		t.Fatalf("projects status = %d, want %d; body=%s", projectsResponse.Code, http.StatusOK, projectsResponse.Body.String())
	}
	var projectsResult struct {
		Data []model.Project `json:"data"`
	}
	if err := json.Unmarshal(projectsResponse.Body.Bytes(), &projectsResult); err != nil {
		t.Fatal(err)
	}
	if len(projectsResult.Data) != 1 || projectsResult.Data[0].ID != allowedProject.ID {
		t.Fatalf("visible projects = %#v, want only %q", projectsResult.Data, allowedProject.ID)
	}

	manageRequest := httptest.NewRequest(http.MethodPost, "/api/environments", bytes.NewBufferString(`{"project_id":"`+allowedProject.ID+`","name":"prod"}`))
	manageRequest.Header.Set("Content-Type", "application/json")
	manageRequest.Header.Set("Authorization", "Bearer "+userToken)
	manageResponse := httptest.NewRecorder()
	router.ServeHTTP(manageResponse, manageRequest)
	if manageResponse.Code != http.StatusForbidden {
		t.Fatalf("ungranted service management status = %d, want %d", manageResponse.Code, http.StatusForbidden)
	}

	updateBody := `{"username":"viewer1","role":"user","project_ids":["` + allowedProject.ID + `"],"permissions":["projects.view"],"disabled":true}`
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/users/"+createdUser.Data.ID, bytes.NewBufferString(updateBody))
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("Authorization", "Bearer "+adminToken)
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("disable user status = %d, want %d; body=%s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}
	meRequest := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+userToken)
	meResponse := httptest.NewRecorder()
	router.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user /me status = %d, want %d", meResponse.Code, http.StatusUnauthorized)
	}

}

func loginToken(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"`+username+`","password":"`+password+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login %q status = %d, want %d; body=%s", username, response.Code, http.StatusOK, response.Body.String())
	}
	var result struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Data.AccessToken
}
