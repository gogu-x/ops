package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/admin/internal/autoopen"
	"github.com/gogu-x/ops/admin/internal/servicetype"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/ops/model"
)

func TestAutoOpenRoutesSaveDraftAndEnable(t *testing.T) {
	ctx := t.Context()
	a := newTestActor(t)
	types := servicetype.NewMemoryRepository()
	if err := types.Create(ctx, model.ServiceType{ID: "game-type", Name: "game"}); err != nil {
		t.Fatal(err)
	}
	instances := instance.NewMemoryRepository()
	if err := instances.Create(ctx, model.ServiceInstance{
		ID: "template", ProjectID: "game-project", EnvironmentID: "beta", Name: "game-1",
		ServiceTypeID: "game-type", HostID: "host", Image: "game:v1", Network: "game-network",
	}); err != nil {
		t.Fatal(err)
	}
	a.app = application.New(nil, nil, nil, types, instances)
	a.autoOpen = autoopen.NewManager(autoopen.NewMemoryRepository(), a.app, nil, nil)
	router := a.newRouter()
	token := loginToken(t, router, "admin", "admin-password")
	request := func(method, path string, input any, status int) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response
	}

	settings := model.AutoOpenSettings{
		Name: "周末开服", TemplateID: "template", NamePrefix: "game-", FirstServerID: 2,
		FirstOpenAt: time.Now().UTC().Truncate(time.Second).Add(time.Hour), Count: 1,
	}
	const policiesPath = "/api/auto-open-policies"
	var response struct {
		Data model.AutoOpenPolicy
	}
	parse := func(recorder *httptest.ResponseRecorder) model.AutoOpenPolicy {
		t.Helper()
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	draft := parse(request(http.MethodPost, policiesPath, settings, http.StatusCreated))
	if draft.ID == "" || draft.Enabled || draft.ProjectID != "game-project" || !draft.NextOpenAt.Equal(settings.FirstOpenAt) {
		t.Fatalf("策略应保存为未启用且继承模板范围: %+v", draft)
	}
	enable := map[string]interface{}{"enabled": true, "revision": draft.Revision}
	path := policiesPath + "/" + draft.ID + "/enabled"
	enabled := parse(request(http.MethodPut, path, enable, http.StatusOK))
	if !enabled.Enabled || enabled.Revision != draft.Revision+1 {
		t.Fatalf("启用后的策略无效: %+v", enabled)
	}
	request(http.MethodPut, path, enable, http.StatusConflict)
	request(http.MethodGet, "/api/auto-open-templates", nil, http.StatusOK)
	request(http.MethodGet, policiesPath, nil, http.StatusOK)
}
