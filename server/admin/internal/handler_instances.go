package internal

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerInstanceRoutes(r *gin.RouterGroup) {
	r.GET("/service-instances", requireAnyPermission(model.PermissionServicesView, model.PermissionProjectsView), a.listServiceInstances)
	r.POST("/service-instances", requirePermission(model.PermissionServicesManage), a.createServiceInstance)
	r.PUT("/service-instances/:id", requirePermission(model.PermissionServicesManage), a.updateServiceInstance)
	r.DELETE("/service-instances/:id", requirePermission(model.PermissionServicesManage), a.deleteServiceInstance)
	r.GET("/service-instances/:id/status", requirePermission(model.PermissionServicesView), a.serviceInstanceStatus)
	r.GET("/service-instances/:id/detail", requirePermission(model.PermissionServicesView), a.serviceInstanceDetail)
	r.GET("/service-instances/:id/resources", requirePermission(model.PermissionServicesView), a.serviceInstanceResourceHistory)
	r.GET("/service-instances/:id/logs", requirePermission(model.PermissionServicesView), a.serviceInstanceLogs)
	r.GET("/service-instances/:id/events", requirePermission(model.PermissionServicesView), a.serviceInstanceEvents)
	r.GET("/service-instances/:id/deployments", requirePermission(model.PermissionServicesView), a.serviceInstanceDeployments)
	r.POST("/service-instances/:id/deployments/:deployment_id/rollback", requirePermission(model.PermissionServicesManage), a.rollbackServiceInstanceDeployment)
	r.POST("/service-instances/:id/deploy", requirePermission(model.PermissionServicesManage), a.deployServiceInstance)
	r.POST("/service-instances/:id/update-image", requirePermission(model.PermissionServicesManage), a.updateServiceInstanceImage)
	r.POST("/service-instances/:id/start", requirePermission(model.PermissionServicesManage), a.startServiceInstance)
	r.POST("/service-instances/:id/stop", requirePermission(model.PermissionServicesManage), a.stopServiceInstance)
	r.POST("/service-instances/:id/restart", requirePermission(model.PermissionServicesManage), a.restartServiceInstance)
	r.DELETE("/service-instances/:id/container", requirePermission(model.PermissionServicesManage), a.removeServiceInstanceContainer)
	r.POST("/service-instances/batch-operations", requirePermission(model.PermissionServicesManage), a.startServiceInstanceBatchOperation)
	r.GET("/service-instance-operations/:id", requirePermission(model.PermissionServicesManage), a.getServiceInstanceOperation)
}

func (a *AdminService) listServiceInstances(c *gin.Context) {
	items, err := a.app.ListInstances(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	user := currentUser(c)
	if user.Role != model.RoleAdmin {
		types, typeErr := a.app.ListServiceTypes(c.Request.Context())
		if typeErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": typeErr.Error()})
			return
		}
		projectsByType := make(map[string]string, len(types))
		for _, item := range types {
			projectsByType[item.ID] = item.ProjectID
		}
		visible := items[:0]
		for _, item := range items {
			if user.HasProject(projectsByType[item.ServiceTypeID]) {
				visible = append(visible, item)
			}
		}
		items = visible
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createServiceInstance(c *gin.Context) {
	var item model.ServiceInstance
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service instance request"})
		return
	}
	if !allowServiceType(c, a, item.ServiceTypeID) {
		return
	}
	created, err := a.app.CreateInstance(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *AdminService) updateServiceInstance(c *gin.Context) {
	var item model.ServiceInstance
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service instance request"})
		return
	}
	item.ID = c.Param("id")
	if !allowInstance(c, a, item.ID) || !allowServiceType(c, a, item.ServiceTypeID) {
		return
	}
	release, available := a.reserveServiceInstance(c, item.ID)
	if !available {
		return
	}
	defer release()
	updated, err := a.app.UpdateInstance(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, model.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *AdminService) deleteServiceInstance(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	err := a.app.DeleteInstance(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) deployServiceInstance(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	result, err := a.app.DeployInstance(c.Request.Context(), c.Param("id"), currentUser(c).Username)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) updateServiceInstanceImage(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	var req model.UpdateServiceInstanceImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request: image is required"})
		return
	}
	result, err := a.app.UpdateInstanceImage(c.Request.Context(), c.Param("id"), req.Image, currentUser(c).Username)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func writeInstanceGatewayError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, model.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, model.ErrHostUnresolved) || errors.Is(err, model.ErrHostProjectConflict) || errors.Is(err, model.ErrDeploymentUnavailable) {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"ok": false, "error": err.Error()})
}

func (a *AdminService) startServiceInstance(c *gin.Context) { a.instanceContainerAction(c, "start") }
func (a *AdminService) stopServiceInstance(c *gin.Context)  { a.instanceContainerAction(c, "stop") }
func (a *AdminService) restartServiceInstance(c *gin.Context) {
	a.instanceContainerAction(c, "restart")
}

func (a *AdminService) instanceContainerAction(c *gin.Context, action string) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	err := a.app.ContainerAction(c.Param("id"), action)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) removeServiceInstanceContainer(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	err := a.app.RemoveContainer(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) serviceInstanceStatus(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	result, err := a.app.InstanceStatus(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceDetail(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	result, err := a.app.InstanceDetail(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceResourceHistory(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "60"))
	result, err := a.app.InstanceResourceHistory(c.Param("id"), limit)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceLogs(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	result, err := a.app.InstanceLogs(c.Param("id"), c.DefaultQuery("tail", "200"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceEvents(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := a.app.InstanceEvents(c.Param("id"), limit)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceDeployments(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if limit <= 0 {
		limit = 30
	} else if limit > 100 {
		limit = 100
	}
	result, err := a.app.InstanceDeployments(c.Param("id"), limit)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) rollbackServiceInstanceDeployment(c *gin.Context) {
	if !allowInstance(c, a, c.Param("id")) {
		return
	}
	release, available := a.reserveServiceInstance(c, c.Param("id"))
	if !available {
		return
	}
	defer release()
	result, err := a.app.RollbackInstance(c.Request.Context(), c.Param("id"), c.Param("deployment_id"), currentUser(c).Username)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) reserveServiceInstance(c *gin.Context, id string) (func(), bool) {
	if a.operations == nil {
		a.operations = newServiceOperationManager(a.app)
	}
	release, err := a.operations.reserve([]string{id})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": err.Error()})
		return nil, false
	}
	return release, true
}

func (a *AdminService) startServiceInstanceBatchOperation(c *gin.Context) {
	var request model.ServiceInstanceBatchOperationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "请选择至少一个服务实例"})
		return
	}
	request.Action = strings.TrimSpace(request.Action)
	request.Image = strings.TrimSpace(request.Image)
	if !supportedBatchAction(request.Action) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "不支持的批量操作"})
		return
	}
	if request.Action == "update_image" && request.Image == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "请填写要更新的镜像版本"})
		return
	}
	if len(request.InstanceIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "单次最多操作 100 个实例"})
		return
	}
	instances, status, err := a.authorizeBatchInstances(c, request.InstanceIDs)
	if err != nil {
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	if a.operations == nil {
		a.operations = newServiceOperationManager(a.app)
	}
	user := currentUser(c)
	operation, err := a.operations.start(user.ID, user.Username, request, instances)
	if err != nil {
		status := http.StatusTooManyRequests
		if errors.Is(err, errInstanceOperationBusy) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "data": operation})
}

func (a *AdminService) authorizeBatchInstances(c *gin.Context, ids []string) ([]model.ServiceInstance, int, error) {
	if len(ids) == 0 {
		return nil, http.StatusBadRequest, errors.New("请选择至少一个服务实例")
	}
	instances, err := a.app.ListInstances(c.Request.Context())
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	types, err := a.app.ListServiceTypes(c.Request.Context())
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	projectByType := make(map[string]string, len(types))
	for _, item := range types {
		projectByType[item.ID] = item.ProjectID
	}
	instanceByID := make(map[string]model.ServiceInstance, len(instances))
	for _, item := range instances {
		instanceByID[item.ID] = item
	}
	user := currentUser(c)
	seen := make(map[string]struct{}, len(ids))
	selected := make([]model.ServiceInstance, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return nil, http.StatusBadRequest, errors.New("实例列表包含重复项")
		}
		seen[id] = struct{}{}
		item, exists := instanceByID[id]
		if !exists {
			return nil, http.StatusNotFound, errors.New("服务实例不存在")
		}
		projectID, exists := projectByType[item.ServiceTypeID]
		if !exists {
			return nil, http.StatusNotFound, errors.New("服务实例所属项目不存在")
		}
		if user.Role != model.RoleAdmin && !user.HasProject(projectID) {
			return nil, http.StatusForbidden, errors.New("批量操作只能包含已授权项目的服务实例")
		}
		selected = append(selected, item)
	}
	return selected, http.StatusOK, nil
}

func (a *AdminService) getServiceInstanceOperation(c *gin.Context) {
	if a.operations == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "操作任务不存在或已过期"})
		return
	}
	operation, ok := a.operations.get(c.Param("id"), currentUser(c).ID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "操作任务不存在或已过期"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": operation})
}
