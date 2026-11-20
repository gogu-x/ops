package autoopen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

var (
	ErrForbidden   = errors.New("无权管理该项目的自动开服策略")
	ErrInvalid     = errors.New("策略配置无效")
	instancePrefix = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,52}$")
)

type GameServices interface {
	GetInstance(context.Context, string) (model.ServiceInstance, error)
	ListStoredInstances(context.Context) ([]model.ServiceInstance, error)
	ListServiceTypes(context.Context) ([]model.ServiceType, error)
	CreateAutoOpenInstance(context.Context, model.ServiceInstance, string, string) (model.ServiceInstance, error)
	InstanceStatus(string) (model.ServiceInstance, error)
	DeployInstance(context.Context, string, ...string) (model.ServiceInstance, error)
}

type Operators interface {
	FindUserByID(context.Context, string) (model.User, error)
}

type Manager struct {
	repo      Repository
	app       GameServices
	operators Operators
	reserve   func([]string) (func(), error)
	cancel    context.CancelFunc
	wake      chan struct{}
}

func NewManager(repo Repository, app GameServices, operators Operators, reserve func([]string) (func(), error)) *Manager {
	return &Manager{repo: repo, app: app, operators: operators, reserve: reserve, wake: make(chan struct{}, 1)}
}

func (m *Manager) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.repo.EnsureIndexes(ctx); err != nil {
		return err
	}
	workerCtx, stop := context.WithCancel(context.Background())
	m.cancel = stop
	go m.run(workerCtx)
	return nil
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) List(ctx context.Context, user model.User) ([]model.AutoOpenPolicy, error) {
	items, err := m.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	visible := make([]model.AutoOpenPolicy, 0, len(items))
	for _, item := range items {
		if user.HasProject(item.ProjectID) {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

func (m *Manager) Templates(ctx context.Context, user model.User) ([]model.AutoOpenTemplate, error) {
	types, err := m.app.ListServiceTypes(ctx)
	if err != nil {
		return nil, err
	}
	var gameID string
	for _, serviceType := range types {
		if serviceType.Name == "game" {
			gameID = serviceType.ID
			break
		}
	}
	options := []model.AutoOpenTemplate{}
	if gameID == "" {
		return options, nil
	}
	instances, err := m.app.ListStoredInstances(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range instances {
		if item.ServiceTypeID == gameID && canManage(user, item.ProjectID) {
			options = append(options, model.AutoOpenTemplate{
				ID: item.ID, Name: item.Name, ProjectID: item.ProjectID,
				EnvironmentID: item.EnvironmentID, Image: item.Image, Parameters: templateParameters(item),
			})
		}
	}
	return options, nil
}

func (m *Manager) Save(ctx context.Context, user model.User, id string, revision int64, settings model.AutoOpenSettings) (model.AutoOpenPolicy, error) {
	item := model.AutoOpenPolicy{
		ID: uuid.NewString(), RecentRuns: []model.AutoOpenRun{},
	}
	if id != "" {
		existing, err := m.authorizedPolicy(ctx, user, id)
		if err != nil {
			return item, err
		}
		if existing.Enabled || existing.Running || len(existing.RecentRuns) != 0 || existing.Revision != revision {
			return item, ErrChanged
		}
		item = existing
	}
	normalizeSettings(&settings)
	if err := validateSettings(settings); err != nil {
		return item, err
	}
	template, err := m.template(ctx, settings.TemplateID, user)
	if err != nil {
		return item, err
	}
	item.AutoOpenSettings = settings
	item.ProjectID, item.EnvironmentID, item.TemplateName = template.ProjectID, template.EnvironmentID, template.Name
	item.NextOpenAt = settings.FirstOpenAt
	if _, err := scheduledInstance(template, item, settings.Count-1, settings.FirstOpenAt.Unix()); err != nil {
		return item, fmt.Errorf("%w：%v", ErrInvalid, err)
	}
	item.Revision++
	item.UpdatedAt = time.Now().UTC()
	if id == "" {
		return item, m.repo.Create(ctx, item)
	}
	return item, m.repo.Replace(ctx, item)
}

func (m *Manager) SetEnabled(ctx context.Context, user model.User, id string, revision int64, enabled bool) (model.AutoOpenPolicy, error) {
	item, err := m.authorizedPolicy(ctx, user, id)
	if err != nil {
		return item, err
	}
	if item.Revision != revision || item.Completed >= item.Count || item.Running {
		return item, ErrChanged
	}
	if item.Enabled == enabled {
		return item, nil
	}
	if enabled {
		item.OperatorID = user.ID
	}
	item.Enabled = enabled
	if err := m.replace(ctx, &item, time.Now().UTC()); err != nil {
		return item, err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return item, nil
}

func normalizeSettings(settings *model.AutoOpenSettings) {
	settings.Name = strings.TrimSpace(settings.Name)
	settings.TemplateID = strings.TrimSpace(settings.TemplateID)
	settings.NamePrefix = strings.TrimSpace(settings.NamePrefix)
	settings.FirstOpenAt = settings.FirstOpenAt.UTC().Truncate(time.Second)
}

func validateSettings(settings model.AutoOpenSettings) error {
	if settings.Name == "" || utf8.RuneCountInString(settings.Name) > 60 || settings.TemplateID == "" {
		return fmt.Errorf("%w：请填写策略名称并选择 Game 模板", ErrInvalid)
	}
	if !instancePrefix.MatchString(settings.NamePrefix) || settings.FirstServerID < 1 || settings.FirstServerID > 65535 {
		return fmt.Errorf("%w：请填写有效的实例名称前缀和起始区服编号", ErrInvalid)
	}
	if settings.FirstOpenAt.IsZero() || settings.FirstOpenAt.Unix() <= 0 ||
		settings.IntervalDays < 0 || settings.IntervalDays > 365 ||
		settings.Count < 1 || settings.Count > 1000 || settings.IntervalDays == 0 && settings.Count != 1 {
		return fmt.Errorf("%w：请填写开服时间和有效的执行次数", ErrInvalid)
	}
	if settings.FirstHostPort < 0 || settings.FirstHostPort > 65535 ||
		settings.FirstHostPort > 0 && settings.FirstHostPort+settings.Count-1 > 65535 {
		return fmt.Errorf("%w：主机发布端口超出 1 到 65535", ErrInvalid)
	}
	return nil
}

func (m *Manager) authorizedPolicy(ctx context.Context, user model.User, id string) (model.AutoOpenPolicy, error) {
	item, err := m.repo.Get(ctx, id)
	if err == nil && !canManage(user, item.ProjectID) {
		err = ErrForbidden
	}
	return item, err
}

func canManage(user model.User, projectID string) bool {
	return !user.Disabled && user.HasProject(projectID) && user.HasPermission(model.PermissionServicesManage)
}

func (m *Manager) template(ctx context.Context, id string, user model.User) (model.ServiceInstance, error) {
	item, err := m.app.GetInstance(ctx, id)
	if err != nil {
		return item, err
	}
	if !canManage(user, item.ProjectID) {
		return item, ErrForbidden
	}
	types, err := m.app.ListServiceTypes(ctx)
	if err != nil {
		return item, err
	}
	for _, serviceType := range types {
		if serviceType.ID == item.ServiceTypeID && serviceType.Name == "game" {
			return item, nil
		}
	}
	return item, fmt.Errorf("%w：请选择 Game 类型的服务实例", ErrInvalid)
}

func (m *Manager) replace(ctx context.Context, item *model.AutoOpenPolicy, now time.Time) error {
	item.Revision++
	item.UpdatedAt = now
	return m.repo.Replace(ctx, *item)
}

func (m *Manager) run(ctx context.Context) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		m.processDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-m.wake:
		}
	}
}

func (m *Manager) processDue(ctx context.Context) {
	work, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	now := time.Now().UTC()
	item, err := m.repo.Due(work, now)
	if err != nil {
		if !errors.Is(err, model.ErrNotFound) && ctx.Err() == nil {
			log.Printf("[ops] 查找待执行的开服策略失败: %v", err)
		}
		return
	}
	item.Running = true
	item.LeaseUntil = now.Add(10 * time.Minute)
	if err := m.replace(work, &item, now); err != nil {
		if !errors.Is(err, ErrChanged) {
			log.Printf("[ops] 领取开服任务 %s 失败: %v", item.ID, err)
		}
		return
	}

	serverID := item.FirstServerID + item.Completed
	run := model.AutoOpenRun{
		ServerID: serverID, OpenAt: item.NextOpenAt,
		InstanceID: scheduledInstanceID(item, serverID), InstanceName: item.NamePrefix + strconv.Itoa(serverID),
	}
	err = m.openServer(work, item, run)
	if ctx.Err() != nil {
		// The next worker reconciles an interrupted run with Docker.
		return
	}

	run.FinishedAt = time.Now().UTC()
	if err != nil {
		run.Error = err.Error()
		item.Enabled = false
	} else {
		item.Completed++
		item.Enabled = item.Completed < item.Count
		if item.Enabled {
			item.NextOpenAt = nextOpenAt(item.NextOpenAt, run.FinishedAt, item.IntervalDays)
		}
	}
	item.Running = false
	item.LeaseUntil = time.Time{}
	item.RecentRuns = append([]model.AutoOpenRun{run}, item.RecentRuns...)
	if len(item.RecentRuns) > 20 {
		item.RecentRuns = item.RecentRuns[:20]
	}
	finish, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := m.replace(finish, &item, run.FinishedAt); err != nil {
		log.Printf("[ops] 保存开服结果 %s/%d 失败: %v", item.ID, serverID, err)
	}
}

func nextOpenAt(previous, now time.Time, days int) time.Time {
	period := time.Duration(days) * 24 * time.Hour
	next := previous.Add(period)
	if !next.After(now) {
		next = next.Add((now.Sub(next)/period + 1) * period)
	}
	return next
}

func (m *Manager) openServer(ctx context.Context, policy model.AutoOpenPolicy, run model.AutoOpenRun) error {
	operator, err := m.operators.FindUserByID(ctx, policy.OperatorID)
	if err != nil {
		return fmt.Errorf("读取策略启用者失败：%w", err)
	}
	if !canManage(operator, policy.ProjectID) {
		return ErrForbidden
	}
	release, err := m.reserve([]string{run.InstanceID})
	if err != nil {
		return err
	}
	defer release()

	item, err := m.app.GetInstance(ctx, run.InstanceID)
	if errors.Is(err, model.ErrNotFound) {
		item, err = m.createInstance(ctx, operator, policy, run)
	}
	if err != nil {
		return err
	}
	if item.AutoOpenPolicyID != policy.ID || item.ProjectID != policy.ProjectID || item.EnvironmentID != policy.EnvironmentID ||
		item.Name != run.InstanceName || gameFlagValue(item, "--server-id") != strconv.Itoa(run.ServerID) ||
		gameFlagValue(item, "--server-open-ts") != strconv.FormatInt(run.OpenAt.Unix(), 10) {
		return errors.New("该区服已被其他策略占用，或生成的实例配置已变更")
	}
	current, err := m.app.InstanceStatus(item.ID)
	if err != nil {
		return err
	}
	switch current.Status {
	case model.StatusRunning:
		if current.ContainerImage != item.Image {
			return errors.New("目标实例运行镜像与开服配置不一致")
		}
		return nil
	case model.StatusNotDeployed:
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := m.app.DeployInstance(ctx, item.ID, operator.Username)
		return err
	default:
		return fmt.Errorf("目标实例已有容器，当前状态 %s，请在服务页检查后重试", current.Status)
	}
}

func (m *Manager) createInstance(ctx context.Context, user model.User, policy model.AutoOpenPolicy, run model.AutoOpenRun) (model.ServiceInstance, error) {
	template, err := m.template(ctx, policy.TemplateID, user)
	if err != nil {
		return template, err
	}
	if template.ProjectID != policy.ProjectID || template.EnvironmentID != policy.EnvironmentID {
		return template, errors.New("模板实例已迁移到其他项目或环境")
	}
	item, err := scheduledInstance(template, policy, policy.Completed, run.OpenAt.Unix())
	if err != nil {
		return item, err
	}
	items, err := m.app.ListStoredInstances(ctx)
	if err != nil {
		return item, err
	}
	for _, existing := range items {
		if existing.ID == run.InstanceID {
			return existing, nil
		}
		nameConflict := existing.Name == item.Name && existing.HostID == item.HostID
		serverConflict := existing.ProjectID == item.ProjectID && existing.EnvironmentID == item.EnvironmentID &&
			existing.ServiceTypeID == item.ServiceTypeID && instanceServerID(existing) == run.ServerID
		if nameConflict || serverConflict {
			return item, fmt.Errorf("实例 %s 已占用目标名称或区服编号 %d", existing.Name, run.ServerID)
		}
	}
	created, err := m.app.CreateAutoOpenInstance(ctx, item, run.InstanceID, policy.ID)
	if errors.Is(err, model.ErrAlreadyExists) {
		return m.app.GetInstance(ctx, run.InstanceID)
	}
	return created, err
}
