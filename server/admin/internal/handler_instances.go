package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerInstanceRoutes(r *gin.RouterGroup) {
	r.GET("/service-instances", a.listServiceInstances)
	r.POST("/service-instances", requireRole("admin"), a.createServiceInstance)
	r.PUT("/service-instances/:id", requireRole("admin"), a.updateServiceInstance)
	r.DELETE("/service-instances/:id", requireRole("admin"), a.deleteServiceInstance)
	r.GET("/service-instances/:id/status", a.serviceInstanceStatus)
	r.GET("/service-instances/:id/detail", a.serviceInstanceDetail)
	r.GET("/service-instances/:id/logs", a.serviceInstanceLogs)
	r.POST("/service-instances/:id/deploy", requireRole("admin"), a.deployServiceInstance)
	r.POST("/service-instances/:id/update-image", requireRole("admin"), a.updateServiceInstanceImage)
	r.POST("/service-instances/:id/start", requireRole("admin"), a.startServiceInstance)
	r.POST("/service-instances/:id/stop", requireRole("admin"), a.stopServiceInstance)
	r.POST("/service-instances/:id/restart", requireRole("admin"), a.restartServiceInstance)
	r.DELETE("/service-instances/:id/container", requireRole("admin"), a.removeServiceInstanceContainer)
}

func (a *AdminService) listServiceInstances(c *gin.Context) {
	items, err := a.app.ListInstances(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createServiceInstance(c *gin.Context) {
	var item model.ServiceInstance
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service instance request"})
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
	result, err := a.app.DeployInstance(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) updateServiceInstanceImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request: image is required"})
		return
	}
	result, err := a.app.UpdateInstanceImage(c.Param("id"), req.Image)
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
	} else if errors.Is(err, model.ErrHostUnresolved) {
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
	err := a.app.ContainerAction(c.Param("id"), action)
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) removeServiceInstanceContainer(c *gin.Context) {
	err := a.app.RemoveContainer(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) serviceInstanceStatus(c *gin.Context) {
	result, err := a.app.InstanceStatus(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceDetail(c *gin.Context) {
	result, err := a.app.InstanceDetail(c.Param("id"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) serviceInstanceLogs(c *gin.Context) {
	result, err := a.app.InstanceLogs(c.Param("id"), c.DefaultQuery("tail", "200"))
	if err != nil {
		writeInstanceGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}
