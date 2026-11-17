package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerRolloutRoutes(r *gin.RouterGroup) {
	r.POST("/rollouts", requirePermission(model.PermissionServicesManage), a.createRollout)
	r.GET("/rollouts", requirePermission(model.PermissionServicesView), a.listRollouts)
	r.GET("/rollouts/:id", requirePermission(model.PermissionServicesView), a.getRollout)
	r.POST("/rollouts/:id/pause", requirePermission(model.PermissionServicesManage), a.pauseRollout)
	r.POST("/rollouts/:id/resume", requirePermission(model.PermissionServicesManage), a.resumeRollout)
	r.POST("/rollouts/:id/retry-failed", requirePermission(model.PermissionServicesManage), a.retryFailedRollout)
}

func (a *AdminService) createRollout(c *gin.Context) {
	var request model.CreateRolloutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "请选择要发布的服务实例"})
		return
	}
	if len(request.InstanceIDs) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "单个滚动发布最多包含 10000 个实例"})
		return
	}
	request.Action = strings.TrimSpace(request.Action)
	request.Image = strings.TrimSpace(request.Image)
	if request.Action != "restart" && request.Action != "update_image" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "滚动发布只支持重启或更新镜像"})
		return
	}
	if request.Action == "update_image" && request.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "请填写目标镜像版本"})
		return
	}
	if request.MaxUnavailablePerGroup == 0 {
		request.MaxUnavailablePerGroup = 1
	}
	if request.MaxParallel == 0 {
		request.MaxParallel = 5
	}
	if request.MaxParallelPerGroup == 0 {
		request.MaxParallelPerGroup = 2
	}
	if request.HealthTimeoutSeconds == 0 {
		request.HealthTimeoutSeconds = 300
	}
	if request.MaxUnavailablePerGroup < 1 || request.MaxUnavailablePerGroup > 1000 || request.MaxParallel < 1 || request.MaxParallel > 50 || request.MaxParallelPerGroup < 1 || request.MaxParallelPerGroup > request.MaxParallel || request.HealthTimeoutSeconds < 10 || request.HealthTimeoutSeconds > 1800 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "不可用数、并发或健康检查超时超出允许范围"})
		return
	}
	if len(request.InstanceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "请选择至少一个实例"})
		return
	}
	serviceTypes, err := a.app.ListServiceTypes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	hosts, err := a.app.ListHosts()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	allInstances, err := a.app.ListInstancesStrict(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	typeByID := make(map[string]model.ServiceType, len(serviceTypes))
	for _, item := range serviceTypes {
		typeByID[item.ID] = item
	}
	hostByID := make(map[string]model.Host, len(hosts))
	for _, item := range hosts {
		hostByID[item.ID] = item
	}
	instanceByID := make(map[string]model.ServiceInstance, len(allInstances))
	for _, item := range allInstances {
		instanceByID[item.ID] = item
	}
	user := currentUser(c)
	selected := make([]model.ServiceInstance, 0, len(request.InstanceIDs))
	seen := make(map[string]struct{}, len(request.InstanceIDs))
	for _, id := range request.InstanceIDs {
		if _, exists := seen[id]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "实例列表包含重复项"})
			return
		}
		seen[id] = struct{}{}
		live, ok := instanceByID[id]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "服务实例不存在"})
			return
		}
		typeConfig, ok := typeByID[live.ServiceTypeID]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "服务实例所属项目不存在"})
			return
		}
		if user.Role != model.RoleAdmin && !user.HasProject(typeConfig.ProjectID) {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "滚动发布只能包含已授权项目的服务实例"})
			return
		}
		selected = append(selected, live)
	}
	if !rolloutHasSingleServiceType(selected) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "一个滚动发布任务只能包含同一服务类型的实例"})
		return
	}
	projectID := ""
	for _, item := range selected {
		typeConfig, ok := typeByID[item.ServiceTypeID]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "实例所属服务类型不存在"})
			return
		}
		if projectID == "" {
			projectID = typeConfig.ProjectID
		} else if projectID != typeConfig.ProjectID {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "一个滚动发布任务只能包含同一项目的实例"})
			return
		}
		if err := validateRolloutProbe(typeConfig.ReadinessProbe); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("服务类型 %s：%v", typeConfig.Name, err)})
			return
		}
		if typeConfig.ReadinessProbe.Type == "tcp" || typeConfig.ReadinessProbe.Type == "http" {
			host, exists := hostByID[item.HostID]
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("实例 %s 的部署主机不存在", item.Name)})
				return
			}
			address := strings.Trim(strings.TrimSpace(host.InternalIP), "[]")
			if address == "" {
				address = strings.Trim(strings.TrimSpace(host.ExternalIP), "[]")
			}
			if address == "" {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("主机 %s 未配置可访问的内网或外网 IP，无法执行健康检查", host.Name)})
				return
			}
			if net.ParseIP(address) == nil {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("主机 %s 的内网/外网 IP 无效，无法执行健康检查", host.Name)})
				return
			}
		}
	}
	groupPlans := make(map[string]model.RolloutGroupPlan)
	for _, item := range allInstances {
		typeConfig, ok := typeByID[item.ServiceTypeID]
		if !ok || typeConfig.ProjectID != projectID {
			continue
		}
		key, name := rolloutGroup(item, typeConfig, hostByID[item.HostID])
		plan, exists := groupPlans[key]
		if !exists {
			plan = model.RolloutGroupPlan{Key: key, Name: name, ProjectID: projectID, MaxUnavailable: request.MaxUnavailablePerGroup, MaxParallel: request.MaxParallelPerGroup}
		}
		if item.Status != model.StatusRunning {
			plan.InitialUnavailable++
		}
		groupPlans[key] = plan
	}
	groups := make([]model.RolloutGroupPlan, 0, len(groupPlans))
	for _, group := range groupPlans {
		groups = append(groups, group)
	}
	items := make([]model.RolloutItem, 0, len(selected))
	for _, item := range selected {
		typeConfig := typeByID[item.ServiceTypeID]
		key, _ := rolloutGroup(item, typeConfig, hostByID[item.HostID])
		target := item.Image
		if request.Action == "update_image" {
			target = request.Image
		}
		items = append(items, model.RolloutItem{
			InstanceID:          item.ID,
			Name:                item.Name,
			HostID:              item.HostID,
			PreviousContainerID: item.ContainerID,
			GroupKey:            key,
			PreviousImage:       item.Image,
			TargetImage:         target,
			ReadinessProbe:      typeConfig.ReadinessProbe,
			InitiallyAvailable:  item.Status == model.StatusRunning,
		})
	}
	job := model.RolloutJob{
		ProjectID:              projectID,
		OwnerID:                currentUser(c).ID,
		Operator:               currentUser(c).Username,
		Action:                 request.Action,
		Image:                  request.Image,
		MaxUnavailablePerGroup: request.MaxUnavailablePerGroup,
		MaxParallel:            request.MaxParallel,
		MaxParallelPerGroup:    request.MaxParallelPerGroup,
		HealthTimeoutSeconds:   request.HealthTimeoutSeconds,
		Groups:                 groups,
	}
	created, err := a.rollouts.Create(job, items)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	detail, err := a.rollouts.Detail(created.ID)
	if err != nil {
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "data": created})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "data": detail})
}

func validateRolloutProbe(probe model.ReadinessProbe) error {
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

func rolloutGroup(instance model.ServiceInstance, serviceType model.ServiceType, host model.Host) (string, string) {
	region := strings.TrimSpace(host.Region)
	if region == "" {
		region = "未标注机房"
	}
	zone := strings.TrimSpace(host.Zone)
	if zone == "" {
		zone = instance.HostID
	}
	group := strings.TrimSpace(instance.RolloutGroup)
	if group == "" {
		group = serviceType.Name
	}
	keyParts := []string{serviceType.ProjectID, serviceType.EnvironmentID, serviceType.ID, region, zone, group}
	encodedKey, _ := json.Marshal(keyParts)
	key := string(encodedKey)
	zoneName := host.Zone
	if strings.TrimSpace(zoneName) == "" {
		zoneName = host.Name
	}
	name := strings.Join([]string{region, zoneName, serviceType.Name, group}, " / ")
	return key, name
}

func (a *AdminService) listRollouts(c *gin.Context) {
	user := currentUser(c)
	projectIDs := user.ProjectIDs
	if user.Role != model.RoleAdmin && len(projectIDs) == 0 {
		projectIDs = []string{"@no-projects"}
	}
	limit := 50
	if parsed, err := strconv.Atoi(c.Query("limit")); err == nil && parsed > 0 && parsed <= 100 {
		limit = parsed
	}
	jobs, err := a.rollouts.List(projectIDs, limit)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": jobs})
}

func (a *AdminService) getRollout(c *gin.Context) {
	detail, ok := a.authorizedRollout(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": detail})
}

func (a *AdminService) authorizedRollout(c *gin.Context) (model.RolloutDetail, bool) {
	detail, err := a.rollouts.Detail(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "滚动发布任务不存在"})
		return model.RolloutDetail{}, false
	}
	user := currentUser(c)
	if user.Role != model.RoleAdmin && !user.HasProject(detail.Job.ProjectID) {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权查看此滚动发布任务"})
		return model.RolloutDetail{}, false
	}
	return detail, true
}

func (a *AdminService) pauseRollout(c *gin.Context) {
	if _, ok := a.authorizedRollout(c); !ok {
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&request)
	if err := a.rollouts.Pause(c.Param("id"), currentUser(c).Username, strings.TrimSpace(request.Reason)); err != nil {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) resumeRollout(c *gin.Context) {
	if _, ok := a.authorizedRollout(c); !ok {
		return
	}
	if err := a.rollouts.Resume(c.Param("id"), currentUser(c).Username); err != nil {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) retryFailedRollout(c *gin.Context) {
	if _, ok := a.authorizedRollout(c); !ok {
		return
	}
	if err := a.rollouts.RetryFailed(c.Param("id"), currentUser(c).Username); err != nil {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func rolloutHasSingleServiceType(instances []model.ServiceInstance) bool {
	if len(instances) == 0 {
		return false
	}
	serviceTypeID := instances[0].ServiceTypeID
	for _, instance := range instances[1:] {
		if instance.ServiceTypeID != serviceTypeID {
			return false
		}
	}
	return true
}
