package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerServiceTypeRoutes(r *gin.RouterGroup) {
	r.GET("/service-types", requireAnyPermission(model.PermissionServicesView, model.PermissionProjectsView), a.listServiceTypes)
	r.POST("/service-types", requirePermission(model.PermissionServicesManage), a.createServiceType)
	r.PUT("/service-types/:id", requirePermission(model.PermissionServicesManage), a.updateServiceType)
	r.DELETE("/service-types/:id", requirePermission(model.PermissionServicesManage), a.deleteServiceType)
}

func (a *AdminService) listServiceTypes(c *gin.Context) {
	items, err := a.app.ListServiceTypes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	user := currentUser(c)
	if user.Role != model.RoleAdmin {
		visible := items[:0]
		for _, item := range items {
			if user.HasProject(item.ProjectID) {
				visible = append(visible, item)
			}
		}
		items = visible
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createServiceType(c *gin.Context) {
	var item model.ServiceType
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service type request"})
		return
	}
	if !allowProject(c, item.ProjectID) {
		return
	}
	created, err := a.app.CreateServiceType(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *AdminService) updateServiceType(c *gin.Context) {
	var item model.ServiceType
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service type request"})
		return
	}
	item.ID = c.Param("id")
	if !allowServiceType(c, a, item.ID) || !allowProject(c, item.ProjectID) {
		return
	}
	updated, err := a.app.UpdateServiceType(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *AdminService) deleteServiceType(c *gin.Context) {
	if !allowServiceType(c, a, c.Param("id")) {
		return
	}
	err := a.app.DeleteServiceType(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
