package internal

import (
	"errors"
	"net/http"
	"strconv"

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
	r.GET("/service-instances/:id/logs", requirePermission(model.PermissionServicesView), a.serviceInstanceLogs)
	r.GET("/service-instances/:id/events", requirePermission(model.PermissionServicesView), a.serviceInstanceEvents)
	r.POST("/service-instances/:id/deploy", requirePermission(model.PermissionServicesManage), a.deployServiceInstance)
	r.POST("/service-instances/:id/update-image", requirePermission(model.PermissionServicesManage), a.updateServiceInstanceImage)
	r.POST("/service-instances/:id/start", requirePermission(model.PermissionServicesManage), a.startServiceInstance)
	r.POST("/service-instances/:id/stop", requirePermission(model.PermissionServicesManage), a.stopServiceInstance)
	r.POST("/service-instances/:id/restart", requirePermission(model.PermissionServicesManage), a.restartServiceInstance)
	r.DELETE("/service-instances/:id/container", requirePermission(model.PermissionServicesManage), a.removeServiceInstanceContainer)
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
	result, err := a.app.DeployInstance(c.Request.Context(), c.Param("id"))
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
	var req model.UpdateServiceInstanceImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request: image is required"})
		return
	}
	result, err := a.app.UpdateInstanceImage(c.Request.Context(), c.Param("id"), req.Image)
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
	} else if errors.Is(err, model.ErrHostUnresolved) || errors.Is(err, model.ErrHostProjectConflict) {
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
