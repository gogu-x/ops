package rollout

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/admin/internal/servicetype"
	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

const (
	rolloutLeaseDuration = 90 * time.Second
	rolloutPollInterval  = 2 * time.Second
)

type Manager struct {
	repo    Repository
	app     *application.Service
	ctx     context.Context
	cancel  context.CancelFunc
	wake    chan struct{}
	started sync.Once
}

func NewManager(repo Repository, app *application.Service) *Manager {
	return &Manager{repo: repo, app: app, wake: make(chan struct{}, 1)}
}

func (m *Manager) Start() error {
	var startErr error
	m.started.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		m.ctx, m.cancel = ctx, cancel
		indexCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		if err := m.repo.EnsureIndexes(indexCtx); err != nil {
			startErr = err
			cancel()
			return
		}
		if err := m.repo.ClearLegacyOperationLocks(indexCtx); err != nil {
			startErr = err
			cancel()
			return
		}
		go m.runLoop(ctx)
	})
	return startErr
}

func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) Create(job model.RolloutJob, items []model.RolloutItem) (model.RolloutJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	job.ID = uuid.NewString()
	job.Status = "queued"
	job.Total = len(items)
	job.CreatedAt = time.Now().UTC()
	job.UpdatedAt = job.CreatedAt
	job.Groups = normalizeRolloutGroups(job.Groups, job.MaxParallelPerGroup, job.MaxUnavailablePerGroup)
	for i := range items {
		items[i].ID = uuid.NewString()
		items[i].JobID = job.ID
		items[i].Status = "pending"
		items[i].UpdatedAt = job.CreatedAt
	}
	event := rolloutEvent(job.ID, "", job.Operator, "created", fmt.Sprintf("创建滚动发布任务，目标 %d 个实例", len(items)))
	if err := m.repo.Create(ctx, job, items, event); err != nil {
		return model.RolloutJob{}, err
	}
	m.signal()
	return job, nil
}

func normalizeRolloutGroups(groups []model.RolloutGroupPlan, maxParallel, maxUnavailable int) []model.RolloutGroupPlan {
	for i := range groups {
		if groups[i].MaxUnavailable < 1 {
			groups[i].MaxUnavailable = maxUnavailable
		}
		if groups[i].MaxParallel < 1 {
			groups[i].MaxParallel = maxParallel
		}
	}
	return groups
}

func (m *Manager) Detail(id string) (model.RolloutDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job, err := m.repo.Get(ctx, id)
	if err != nil {
		return model.RolloutDetail{}, err
	}
	items, err := m.repo.Items(ctx, id)
	if err != nil {
		return model.RolloutDetail{}, err
	}
	events, err := m.repo.Events(ctx, id, 300)
	if err != nil {
		return model.RolloutDetail{}, err
	}
	job.Total = len(items)
	job.Completed, job.Succeeded, job.Failed = 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case "succeeded":
			job.Completed++
			job.Succeeded++
		case "failed":
			job.Completed++
			job.Failed++
		}
	}
	return model.RolloutDetail{Job: job, Items: items, Events: events}, nil
}

func (m *Manager) List(projectIDs []string, limit int) ([]model.RolloutJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	jobs, err := m.repo.List(ctx, projectIDs, limit)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		items, err := m.repo.Items(ctx, jobs[i].ID)
		if err != nil {
			return nil, err
		}
		jobs[i].Total = len(items)
		jobs[i].Completed, jobs[i].Succeeded, jobs[i].Failed = 0, 0, 0
		for _, item := range items {
			switch item.Status {
			case "succeeded":
				jobs[i].Completed++
				jobs[i].Succeeded++
			case "failed":
				jobs[i].Completed++
				jobs[i].Failed++
			}
		}
	}
	return jobs, nil
}

func (m *Manager) Pause(id, operator, reason string) error {
	if err := m.repo.SetJobStatus(context.Background(), id, "paused", reason, "queued", "running"); err != nil {
		if errors.Is(err, errRolloutStateChanged) {
			return errors.New("只有排队或运行中的任务可以暂停")
		}
		return err
	}
	_ = m.repo.AppendEvent(context.Background(), rolloutEvent(id, "", operator, "paused", firstNonEmpty(reason, "操作员暂停任务")))
	return nil
}

func (m *Manager) Resume(id, operator string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	owner := uuid.NewString()
	err := m.repo.LeaseInactiveJob(ctx, id, owner, time.Now().UTC().Add(10*time.Second), "paused")
	if err != nil {
		if errors.Is(err, errRolloutStateChanged) {
			return errors.New("只有暂停且已结束当前实例操作的任务可以恢复")
		}
		return err
	}
	defer m.releaseLease(id, owner, time.Time{})
	if err := m.repo.SetJobStatus(ctx, id, "queued", "", "paused"); err != nil {
		return err
	}
	_ = m.repo.AppendEvent(ctx, rolloutEvent(id, "", operator, "resumed", "恢复滚动发布任务"))
	m.signal()
	return nil
}

func (m *Manager) RetryFailed(id, operator string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	owner := uuid.NewString()
	err := m.repo.LeaseInactiveJob(ctx, id, owner, time.Now().UTC().Add(4*time.Minute), "paused", "partial", "failed")
	if err != nil {
		if errors.Is(err, errRolloutStateChanged) {
			return errors.New("任务需暂停或完成，且已结束当前实例操作才能重试失败项")
		}
		return err
	}
	defer m.releaseLease(id, owner, time.Time{})
	job, err := m.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	items, err := m.repo.Items(ctx, id)
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for _, item := range items {
		if item.Status == "failed" {
			ids = append(ids, item.InstanceID)
		}
	}
	if len(ids) == 0 {
		return errors.New("没有失败项可重试")
	}
	lockOwner := rolloutLockOwner(id, owner)
	if err := m.repo.AcquireInstanceLocks(ctx, lockOwner, ids); err != nil {
		return err
	}
	defer m.releaseInstanceLocks(lockOwner)
	liveItems, err := m.app.ListInstancesStrict(ctx)
	if err != nil {
		return fmt.Errorf("读取实例状态以安全重试失败项: %w", err)
	}
	liveByID := make(map[string]model.ServiceInstance, len(liveItems))
	for _, live := range liveItems {
		liveByID[live.ID] = live
	}
	for _, item := range items {
		if item.Status != "failed" {
			continue
		}
		current, exists := liveByID[item.InstanceID]
		eventType, eventMessage := "retry_queued", "重新排队执行失败实例"
		if exists && rolloutActionApplied(job, item, current, nil) {
			item.Status = "health_check"
			eventType, eventMessage = "retry_resume_health_check", "目标容器已存在，只恢复就绪检查"
		} else {
			item.Status = "pending"
		}
		item.Error = ""
		item.FinishedAt = time.Time{}
		item.UpdatedAt = time.Now().UTC()
		if err := m.repo.UpdateItem(ctx, item); err != nil {
			return err
		}
		_ = m.repo.AppendEvent(ctx, rolloutEvent(id, item.InstanceID, operator, eventType, eventMessage))
	}
	if err := m.repo.SetJobStatus(ctx, id, "queued", "", job.Status); err != nil {
		return err
	}
	_ = m.repo.AppendEvent(ctx, rolloutEvent(id, "", operator, "retry_failed", fmt.Sprintf("重新排队 %d 个失败实例", len(ids))))
	m.signal()
	return nil
}

func (m *Manager) runLoop(ctx context.Context) {
	ticker := time.NewTicker(rolloutPollInterval)
	defer ticker.Stop()
	for {
		m.claimAndRun(ctx)
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
	}
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) claimAndRun(ctx context.Context) {
	claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	job, ok, err := m.repo.ClaimNext(claimCtx, uuid.NewString(), time.Now().UTC(), rolloutLeaseDuration)
	cancel()
	if err != nil {
		log.Printf("[ops] claim rollout job: %v", err)
		return
	}
	if !ok {
		return
	}
	m.process(ctx, job)
}

func (m *Manager) refreshPendingPlan(ctx context.Context, job *model.RolloutJob, items []model.RolloutItem) error {
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item.Attempt > 0 || item.Status != "pending" {
			return nil
		}
	}
	// A job may have been queued while another rollout was changing availability.
	types, err := m.app.ListServiceTypes(ctx)
	if err != nil {
		return err
	}
	hosts, err := m.app.ListHosts()
	if err != nil {
		return err
	}
	live, err := m.app.ListInstancesStrict(ctx)
	if err != nil {
		return err
	}
	typeByID := make(map[string]model.ServiceType, len(types))
	for _, serviceType := range types {
		typeByID[serviceType.ID] = serviceType
	}
	hostByID := make(map[string]model.Host, len(hosts))
	for _, host := range hosts {
		hostByID[host.ID] = host
	}
	liveByID := make(map[string]model.ServiceInstance, len(live))
	for _, instance := range live {
		liveByID[instance.ID] = instance
	}
	groups, _ := BuildPlan(model.CreateRolloutRequest{}, job.ProjectID, nil, live, typeByID, hostByID)
	unavailable := make(map[string]int, len(groups))
	for _, group := range groups {
		unavailable[group.Key] = group.InitialUnavailable
	}
	job.Groups = append([]model.RolloutGroupPlan(nil), job.Groups...)
	groupsChanged := false
	for i := range job.Groups {
		group := &job.Groups[i]
		if count := unavailable[group.Key]; group.InitialUnavailable != count {
			group.InitialUnavailable = count
			groupsChanged = true
		}
	}
	for i := range items {
		item := &items[i]
		instance, exists := liveByID[item.InstanceID]
		if !exists {
			return fmt.Errorf("实例 %s 已不存在", item.Name)
		}
		serviceType, exists := typeByID[instance.ServiceTypeID]
		key, _ := rolloutGroup(instance, serviceType, hostByID[instance.HostID])
		if !exists || key != item.GroupKey {
			return fmt.Errorf("实例 %s 的发布分组已变更，请重新创建任务", item.Name)
		}
		available := instance.Status == model.StatusRunning
		if item.InitiallyAvailable == available && item.PreviousContainerID == instance.ContainerID && item.PreviousImage == instance.Image {
			continue
		}
		item.InitiallyAvailable = available
		item.PreviousContainerID = instance.ContainerID
		item.PreviousImage = instance.Image
		if err := m.repo.UpdateItem(ctx, *item); err != nil {
			return err
		}
	}
	if groupsChanged {
		return m.repo.SetJobGroups(ctx, job.ID, job.Groups)
	}
	return nil
}

func (m *Manager) process(ctx context.Context, job model.RolloutJob) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workCtx, cancelWork := context.WithCancel(jobCtx)
	defer cancelWork()
	heartbeatDone := make(chan struct{})
	var retryAt time.Time
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if err := m.repo.RenewLease(jobCtx, job.ID, job.LeaseOwner, time.Now().UTC().Add(rolloutLeaseDuration)); err != nil {
					log.Printf("[ops] renew rollout lease %s: %v", job.ID, err)
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		<-heartbeatDone
		m.releaseLease(job.ID, job.LeaseOwner, retryAt)
	}()

	items, err := m.repo.Items(jobCtx, job.ID)
	if err != nil {
		m.pause(job.ID, job.Operator, "读取任务实例失败: "+err.Error())
		return
	}
	if len(items) > 0 {
		ids := make([]string, 0, len(items))
		groups := make(map[string]struct{})
		for _, item := range items {
			ids = append(ids, item.InstanceID)
			if _, exists := groups[item.GroupKey]; !exists && item.GroupKey != "" {
				groups[item.GroupKey] = struct{}{}
				ids = append(ids, "@rollout-group:"+item.GroupKey)
			}
		}
		lockOwner := rolloutLockOwner(job.ID, job.LeaseOwner)
		if err := m.repo.AcquireInstanceLocks(jobCtx, lockOwner, ids); err != nil {
			if errors.Is(err, errRolloutLocked) {
				if m.repo.SetJobStatus(jobCtx, job.ID, "queued", "", "running") == nil {
					retryAt = time.Now().UTC().Add(rolloutPollInterval)
				}
			} else if jobCtx.Err() == nil {
				m.pause(job.ID, job.Operator, "锁定发布实例失败: "+err.Error())
			}
			return
		}
		defer m.releaseInstanceLocks(lockOwner)
	}
	var workers sync.WaitGroup
	defer func() {
		cancelWork()
		workers.Wait()
	}()
	current, err := m.repo.Get(jobCtx, job.ID)
	if err != nil || current.Status != "running" || current.LeaseOwner != job.LeaseOwner {
		return
	}
	if err := m.refreshPendingPlan(jobCtx, &job, items); err != nil {
		if jobCtx.Err() == nil {
			m.pause(job.ID, job.Operator, "刷新发布前实例状态失败: "+err.Error())
		}
		return
	}
	// A process may have stopped after marking an item active. Requeue those items;
	// reconcile the Docker state before deciding whether to re-run the operation.
	recoveredChecks := make([]model.RolloutItem, 0)
	for i := range items {
		if items[i].Status != "running" && items[i].Status != "health_check" {
			continue
		}
		current, statusErr := m.app.InstanceStatus(items[i].InstanceID)
		applied := rolloutActionApplied(job, items[i], current, statusErr)
		if applied {
			items[i].Status = "health_check"
			recoveredChecks = append(recoveredChecks, items[i])
			_ = m.repo.AppendEvent(jobCtx, rolloutEvent(job.ID, items[i].InstanceID, job.Operator, "recovered_health_check", "检测到目标容器已创建，恢复就绪检查"))
		} else {
			items[i].Status = "pending"
			items[i].Error = ""
			_ = m.repo.AppendEvent(jobCtx, rolloutEvent(job.ID, items[i].InstanceID, job.Operator, "recovered_retry", "目标容器尚未确认，恢复实例操作"))
		}
		items[i].UpdatedAt = time.Now().UTC()
		if err := m.repo.UpdateItem(jobCtx, items[i]); err != nil {
			m.pause(job.ID, job.Operator, "恢复中断实例失败: "+err.Error())
			return
		}
	}
	groupByKey := make(map[string]model.RolloutGroupPlan, len(job.Groups))
	for _, group := range job.Groups {
		groupByKey[group.Key] = group
	}
	for _, item := range recoveredChecks {
		workers.Add(1)
		go func(item model.RolloutItem) {
			defer workers.Done()
			m.verifyItem(workCtx, job, item)
		}(item)
	}
	for {
		if jobCtx.Err() != nil {
			return
		}
		current, err := m.repo.Get(jobCtx, job.ID)
		if err != nil || current.Status != "running" || current.LeaseOwner != job.LeaseOwner {
			return
		}
		items, err = m.repo.Items(jobCtx, job.ID)
		if err != nil {
			m.pause(job.ID, job.Operator, "读取任务进度失败: "+err.Error())
			return
		}
		m.refreshProgress(job.ID, items)
		globalActive := 0
		groupActive := make(map[string]int)
		groupUnavailable := make(map[string]int, len(groupByKey))
		for key, group := range groupByKey {
			groupUnavailable[key] = group.InitialUnavailable
		}
		pending := make([]model.RolloutItem, 0)
		unfinished := false
		for _, item := range items {
			switch item.Status {
			case "running", "health_check":
				globalActive++
				groupActive[item.GroupKey]++
				unfinished = true
				if item.InitiallyAvailable {
					groupUnavailable[item.GroupKey]++
				}
			case "failed":
				if item.InitiallyAvailable {
					groupUnavailable[item.GroupKey]++
				}
			case "pending":
				pending = append(pending, item)
				unfinished = true
			case "succeeded":
				if !item.InitiallyAvailable && groupUnavailable[item.GroupKey] > 0 {
					groupUnavailable[item.GroupKey]--
				}
			}
		}
		if !unfinished {
			status := "succeeded"
			for _, item := range items {
				if item.Status == "failed" {
					status = "partial"
					if allRolloutItemsFailed(items) {
						status = "failed"
					}
					break
				}
			}
			m.finish(job.ID, job.Operator, status)
			return
		}
		started := 0
		for _, item := range pending {
			if globalActive >= current.MaxParallel {
				break
			}
			group, ok := groupByKey[item.GroupKey]
			if !ok {
				m.pause(job.ID, job.Operator, "任务分组快照缺失: "+item.GroupKey)
				break
			}
			if groupActive[item.GroupKey] >= group.MaxParallel {
				continue
			}
			additionalUnavailable := 0
			if item.InitiallyAvailable {
				additionalUnavailable = 1
			}
			if additionalUnavailable > 0 && groupUnavailable[item.GroupKey]+additionalUnavailable > group.MaxUnavailable {
				continue
			}
			item.Status = "running"
			item.Attempt++
			item.Error = ""
			item.StartedAt = time.Now().UTC()
			item.UpdatedAt = item.StartedAt
			if err := m.repo.UpdateItem(jobCtx, item); err != nil {
				m.pause(job.ID, job.Operator, "保存实例开始状态失败: "+err.Error())
				break
			}
			if err := m.repo.AppendEvent(jobCtx, rolloutEvent(job.ID, item.InstanceID, job.Operator, "instance_started", fmt.Sprintf("开始第 %d 次执行", item.Attempt))); err != nil {
				item.Status = "failed"
				item.Error = "写入审计记录失败: " + err.Error()
				item.FinishedAt = time.Now().UTC()
				_ = m.repo.UpdateItem(jobCtx, item)
				m.pause(job.ID, job.Operator, item.Error)
				break
			}
			globalActive++
			groupActive[item.GroupKey]++
			groupUnavailable[item.GroupKey] += additionalUnavailable
			started++
			workers.Add(1)
			go func(item model.RolloutItem) {
				defer workers.Done()
				m.runItem(workCtx, current, item)
			}(item)
		}
		if started == 0 && globalActive == 0 && len(pending) > 0 {
			m.pause(job.ID, job.Operator, "组内已有不可用实例达到上限，或并发策略无法继续；恢复服务后可续跑")
			return
		}
		select {
		case <-jobCtx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func allRolloutItemsFailed(items []model.RolloutItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Status != "failed" {
			return false
		}
	}
	return true
}

func rolloutActionApplied(job model.RolloutJob, item model.RolloutItem, current model.ServiceInstance, statusErr error) bool {
	if statusErr != nil || current.Status != model.StatusRunning || current.ContainerID == "" || current.ContainerID == item.PreviousContainerID {
		return false
	}
	return (job.Action != "update_image" || current.ContainerImage == item.TargetImage) &&
		servicetype.ParameterValuesMatch(current.Params, item.ParameterValues)
}

func (m *Manager) runItem(ctx context.Context, job model.RolloutJob, item model.RolloutItem) {
	if ctx.Err() != nil {
		return
	}
	var err error
	switch job.Action {
	case "restart":
		err = m.app.RestartInstanceWithParameters(ctx, item.InstanceID, job.Operator, item.ParameterValues)
	case "update_image":
		_, err = m.app.UpdateInstanceImageWithParameters(ctx, item.InstanceID, item.TargetImage, job.Operator, item.ParameterValues)
	default:
		err = fmt.Errorf("unsupported rollout action %q", job.Action)
	}
	if err != nil {
		if ctx.Err() == nil {
			m.failItem(job, item, "执行操作失败: "+err.Error())
		}
		return
	}
	item.Status = "health_check"
	item.UpdatedAt = time.Now().UTC()
	if err := m.repo.UpdateItem(context.Background(), item); err != nil {
		m.pause(job.ID, job.Operator, "保存健康检查阶段失败: "+err.Error())
		return
	}
	_ = m.repo.AppendEvent(context.Background(), rolloutEvent(job.ID, item.InstanceID, job.Operator, "health_check_started", "容器操作成功，等待就绪检查"))
	m.verifyItem(ctx, job, item)
}

func (m *Manager) verifyItem(ctx context.Context, job model.RolloutJob, item model.RolloutItem) {
	deadline := time.NewTimer(time.Duration(job.HealthTimeoutSeconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	consecutiveReady := 0
	lastReason := "等待第一次健康检查"
	for {
		if ctx.Err() != nil {
			return
		}
		ready, reason, probeErr := m.app.InstanceReadiness(ctx, item.InstanceID, item.ReadinessProbe)
		if ctx.Err() != nil {
			return
		}
		if probeErr != nil {
			reason = probeErr.Error()
		}
		lastReason = reason
		if ready {
			consecutiveReady++
			if consecutiveReady >= 2 {
				item.Status = "succeeded"
				item.Error = ""
				item.FinishedAt = time.Now().UTC()
				item.UpdatedAt = item.FinishedAt
				if err := m.repo.UpdateItem(context.Background(), item); err != nil {
					m.pause(job.ID, job.Operator, "保存实例成功状态失败: "+err.Error())
					return
				}
				_ = m.repo.AppendEvent(context.Background(), rolloutEvent(job.ID, item.InstanceID, job.Operator, "instance_succeeded", "就绪检查连续通过，实例发布成功"))
				m.refreshProgress(job.ID, nil)
				return
			}
		} else {
			consecutiveReady = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			m.failItem(job, item, "健康检查超时: "+lastReason)
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) failItem(job model.RolloutJob, item model.RolloutItem, reason string) {
	pauseErr := m.repo.SetJobStatus(context.Background(), job.ID, "paused", reason, "running")
	item.Status = "failed"
	item.Error = reason
	item.FinishedAt = time.Now().UTC()
	item.UpdatedAt = item.FinishedAt
	if err := m.repo.UpdateItem(context.Background(), item); err != nil {
		log.Printf("[ops] save failed rollout item %s: %v", item.InstanceID, err)
	}
	_ = m.repo.AppendEvent(context.Background(), rolloutEvent(job.ID, item.InstanceID, job.Operator, "instance_failed", reason))
	if pauseErr == nil {
		_ = m.repo.AppendEvent(context.Background(), rolloutEvent(job.ID, "", job.Operator, "auto_paused", "实例失败，任务自动暂停: "+reason))
	}
	m.refreshProgress(job.ID, nil)
}

func (m *Manager) pause(id, operator, reason string) {
	if err := m.repo.SetJobStatus(context.Background(), id, "paused", reason, "running"); err != nil {
		return
	}
	_ = m.repo.AppendEvent(context.Background(), rolloutEvent(id, "", operator, "auto_paused", reason))
}

func (m *Manager) finish(id, operator, status string) {
	if err := m.repo.SetJobStatus(context.Background(), id, status, "", "running"); err != nil {
		log.Printf("[ops] finish rollout job %s: %v", id, err)
		return
	}
	_ = m.repo.AppendEvent(context.Background(), rolloutEvent(id, "", operator, status, "滚动发布任务完成"))
}

func (m *Manager) releaseLease(id, owner string, nextClaim time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.repo.ReleaseLease(ctx, id, owner, nextClaim); err != nil {
		log.Printf("[ops] release rollout lease %s: %v", id, err)
	}
}

func (m *Manager) releaseInstanceLocks(owner string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.repo.ReleaseInstanceLocks(ctx, owner); err != nil {
		log.Printf("[ops] release rollout instance locks %s: %v", owner, err)
	}
}

func (m *Manager) refreshProgress(id string, supplied []model.RolloutItem) {
	items := supplied
	if items == nil {
		var err error
		items, err = m.repo.Items(context.Background(), id)
		if err != nil {
			return
		}
	}
	completed, succeeded, failed := 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case "succeeded":
			completed++
			succeeded++
		case "failed":
			completed++
			failed++
		}
	}
	_ = m.repo.SetJobProgress(context.Background(), id, len(items), completed, succeeded, failed)
}

func rolloutEvent(jobID, instanceID, operator, eventType, message string) model.RolloutAuditEvent {
	return model.RolloutAuditEvent{ID: uuid.NewString(), JobID: jobID, InstanceID: instanceID, Operator: operator, Type: eventType, Message: message, CreatedAt: time.Now().UTC()}
}

func rolloutLockOwner(id, leaseOwner string) string { return "rollout:" + id + ":" + leaseOwner }

func sortedRolloutItems(items []model.RolloutItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].GroupKey == items[j].GroupKey {
			return items[i].Name < items[j].Name
		}
		return items[i].GroupKey < items[j].GroupKey
	})
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

var ErrInvalidTemplate = errors.New("无效的策略模板")

func (m *Manager) ListTemplates(ctx context.Context) ([]model.RolloutTemplate, error) {
	return m.repo.ListTemplates(ctx)
}

func (m *Manager) CreateTemplate(ctx context.Context, item model.RolloutTemplate) (model.RolloutTemplate, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" || utf8.RuneCountInString(item.Name) > 60 {
		return model.RolloutTemplate{}, fmt.Errorf("%w: %s", ErrInvalidTemplate, "模板名称需为 1 到 60 个字符")
	}
	valid := false
	switch item.Kind {
	case "release":
		valid = item.MaxUnavailable >= 1 && item.MaxUnavailable <= 1000 &&
			item.MaxParallel >= 1 && item.MaxParallel <= 50 &&
			item.MaxParallelPerGroup >= 1 && item.MaxParallelPerGroup <= item.MaxParallel
		item.Probe, item.Timeout = nil, 0
	case "health":
		valid = item.Probe != nil && ValidateProbe(*item.Probe) == nil && item.Timeout >= 10 && item.Timeout <= 1800
		item.MaxUnavailable, item.MaxParallel, item.MaxParallelPerGroup = 0, 0, 0
	}
	if !valid {
		return model.RolloutTemplate{}, fmt.Errorf("%w: %s", ErrInvalidTemplate, "策略参数无效或超出允许范围")
	}
	item.ID = uuid.NewString()
	if err := m.repo.CreateTemplate(ctx, item); err != nil {
		return model.RolloutTemplate{}, err
	}
	return item, nil
}

func ValidateProbe(probe model.ReadinessProbe) error {
	switch probe.Type {
	case "docker", "tcp":
	case "http":
		if probe.Path != "" && !strings.HasPrefix(probe.Path, "/") {
			return errors.New("HTTP 探针路径必须以 / 开头")
		}
	case "":
		return errors.New("尚未配置就绪检查，请在服务类型中设置 Docker、TCP 或 HTTP 探针")
	default:
		return errors.New("就绪检查类型无效")
	}
	if (probe.Type == "tcp" || probe.Type == "http") && (probe.Port < 1 || probe.Port > 65535) {
		return errors.New("TCP/HTTP 探针端口必须在 1 到 65535 之间")
	}
	return nil
}
