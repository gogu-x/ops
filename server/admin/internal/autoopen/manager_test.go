package autoopen

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogu-x/ops/admin/internal/users"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/ops/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type testGameServices struct {
	instance.Repository
	mu        sync.Mutex
	running   map[string]model.ServiceInstance
	deployErr error
	deploys   int
}

func (s *testGameServices) GetInstance(ctx context.Context, id string) (model.ServiceInstance, error) {
	return s.Get(ctx, id)
}

func (s *testGameServices) ListStoredInstances(ctx context.Context) ([]model.ServiceInstance, error) {
	return s.List(ctx)
}

func (*testGameServices) ListServiceTypes(context.Context) ([]model.ServiceType, error) {
	return []model.ServiceType{{ID: "type-game", Name: "game"}}, nil
}

func (s *testGameServices) CreateAutoOpenInstance(ctx context.Context, item model.ServiceInstance, id, policyID string) (model.ServiceInstance, error) {
	item.ID, item.AutoOpenPolicyID = id, policyID
	return item, s.Create(ctx, item)
}

func (s *testGameServices) InstanceStatus(id string) (model.ServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, exists := s.running[id]; exists {
		return item, nil
	}
	return model.ServiceInstance{Status: model.StatusNotDeployed}, nil
}

func (s *testGameServices) DeployInstance(ctx context.Context, id string, _ ...string) (model.ServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deploys++
	if s.deployErr != nil {
		return model.ServiceInstance{}, s.deployErr
	}
	item, err := s.Get(ctx, id)
	if err != nil {
		return item, err
	}
	item.Status, item.ContainerImage = model.StatusRunning, item.Image
	s.running[id] = item
	return item, nil
}

func testManager(t *testing.T) (*Manager, *testGameServices, model.User, model.AutoOpenSettings) {
	t.Helper()
	ctx := t.Context()
	repo := NewMemoryRepository()
	app := &testGameServices{Repository: instance.NewMemoryRepository(), running: make(map[string]model.ServiceInstance)}
	template := model.ServiceInstance{
		ID: "template", Name: "game-1", ServiceTypeID: "type-game", ProjectID: "p", EnvironmentID: "e", HostID: "docker",
		Image: "gogs-game:latest", Network: "game-network", PortMapping: "9001:9901",
		Params:  []model.ServiceParam{{Flag: "--server-id", Value: "1"}, {Flag: "--gconf-db", Value: "gs_conf_bate"}},
		EnvText: "NATS_URL=nats://nats:4222\n--grpc-host=game-1\n--game-base-port 9900\n--server-open-ts 1234\n-p 9002:9901",
	}
	if err := app.Create(ctx, template); err != nil {
		t.Fatal(err)
	}
	operators := users.NewMemoryRepository()
	user := model.User{ID: "operator", Username: "operator", Role: model.RoleAdmin}
	if err := operators.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(repo, app, operators, func([]string) (func(), error) {
		return func() {}, nil
	})
	settings := model.AutoOpenSettings{
		Name: "定时开服", TemplateID: template.ID, NamePrefix: "game-", FirstServerID: 2,
		FirstOpenAt: time.Now().UTC().Truncate(time.Second).Add(-time.Hour), Count: 1, FirstHostPort: 12000,
	}
	return manager, app, user, settings
}

func TestTimedAutoOpenCreatesSingleGameFromTemplate(t *testing.T) {
	manager, app, user, settings := testManager(t)
	ctx := t.Context()
	policy, err := manager.Save(ctx, user, "", 0, settings)
	if err != nil {
		t.Fatal(err)
	}
	manager.processDue(ctx)
	if app.deploys != 0 {
		t.Fatal("未启用的策略触发了部署")
	}
	policy, err = manager.SetEnabled(ctx, user, policy.ID, policy.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() { manager.processDue(ctx) })
	}
	workers.Wait()
	policy, err = manager.repo.Get(ctx, policy.ID)
	if err != nil || policy.Enabled || policy.Completed != 1 || app.deploys != 1 || len(policy.RecentRuns) != 1 {
		t.Fatalf("执行结果不符合预期: %+v, 部署次数 %d, 错误 %v", policy, app.deploys, err)
	}
	run := policy.RecentRuns[0]
	created, err := app.GetInstance(ctx, run.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "game-2" || created.PortMapping != "12000:9902" || instanceServerID(created) != 2 ||
		gameFlagValue(created, "--server-open-ts") != strconv.FormatInt(settings.FirstOpenAt.Unix(), 10) ||
		gameFlagValue(created, "--grpc-host") != "game-2" || gameFlagValue(created, "--gconf-db") != "gs_conf_bate" ||
		strings.Contains(created.EnvText, "9002:9901") || !strings.Contains(created.EnvText, "NATS_URL=") {
		t.Fatalf("生成的实例配置不符合预期: %+v", created)
	}
}

func TestRetryUsesSameServerAndAvoidsRedeployWhenContainerAlreadyExists(t *testing.T) {
	manager, app, user, settings := testManager(t)
	ctx := t.Context()
	settings.IntervalDays, settings.Count = 1, 2
	policy, err := manager.Save(ctx, user, "", 0, settings)
	if err != nil {
		t.Fatal(err)
	}
	policy, err = manager.SetEnabled(ctx, user, policy.ID, policy.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	app.deployErr = errors.New("deploy timeout")
	manager.processDue(ctx)
	policy, err = manager.repo.Get(ctx, policy.ID)
	if err != nil || policy.Enabled || policy.Completed != 0 || len(policy.RecentRuns) != 1 || policy.RecentRuns[0].Error == "" {
		t.Fatalf("失败的策略应停用并保留本次开服记录: %+v, %v", policy, err)
	}
	firstRun := policy.RecentRuns[0]
	created, err := app.GetInstance(ctx, firstRun.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	created.Status, created.ContainerImage = model.StatusRunning, created.Image
	app.running[created.ID] = created
	policy, err = manager.SetEnabled(ctx, user, policy.ID, policy.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	manager.processDue(ctx)
	policy, err = manager.repo.Get(ctx, policy.ID)
	if err != nil || !policy.Enabled || policy.Completed != 1 || app.deploys != 1 || len(policy.RecentRuns) != 2 {
		t.Fatalf("重试不应重新部署已运行的实例: %+v, 部署次数 %d, 错误 %v", policy, app.deploys, err)
	}
	if policy.RecentRuns[0].InstanceID != firstRun.InstanceID || policy.RecentRuns[0].Error != "" ||
		!policy.NextOpenAt.Equal(settings.FirstOpenAt.Add(24*time.Hour)) {
		t.Fatalf("重试未延用区服编号或下次开服时间错误: %+v", policy)
	}
}

func TestAutoOpenRejectsExistingServerIDAndKeepsPolicyScoped(t *testing.T) {
	manager, app, admin, settings := testManager(t)
	ctx := t.Context()
	viewer := model.User{ID: "other", Role: model.RoleUser, ProjectIDs: []string{"another"}, Permissions: []string{model.PermissionServicesManage}}
	if _, err := manager.Save(ctx, viewer, "", 0, settings); !errors.Is(err, ErrForbidden) {
		t.Fatalf("跨项目引用模板应失败, 实际 %v", err)
	}
	existing := model.ServiceInstance{
		ID: "manual", Name: "manual-game", ServiceTypeID: "type-game", ProjectID: "p", EnvironmentID: "e",
		EnvText: "--server-id=002",
	}
	if err := app.Create(ctx, existing); err != nil {
		t.Fatal(err)
	}
	policy, err := manager.Save(ctx, admin, "", 0, settings)
	if err != nil {
		t.Fatal(err)
	}
	policy, err = manager.SetEnabled(ctx, admin, policy.ID, policy.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	manager.processDue(ctx)
	policy, err = manager.repo.Get(ctx, policy.ID)
	if err != nil || policy.Enabled || app.deploys != 0 || len(policy.RecentRuns) != 1 || !strings.Contains(policy.RecentRuns[0].Error, "已占用") {
		t.Fatalf("重复区服编号应阻止部署: %+v, %v", policy, err)
	}
}

func TestAutoOpenPolicyBSONKeepsScheduleAndLeaseFields(t *testing.T) {
	openAt := time.Now().UTC().Truncate(time.Millisecond)
	policy := model.AutoOpenPolicy{
		AutoOpenSettings: model.AutoOpenSettings{Name: "schedule", FirstOpenAt: openAt}, NextOpenAt: openAt, Revision: 3,
	}
	data, err := bson.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip model.AutoOpenPolicy
	if err := bson.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Name != policy.Name || !roundTrip.FirstOpenAt.Equal(openAt) || !roundTrip.NextOpenAt.Equal(openAt) ||
		roundTrip.Revision != 3 || !roundTrip.LeaseUntil.IsZero() {
		t.Fatalf("策略 BSON 往返数据不匹配: %+v", roundTrip)
	}
}
