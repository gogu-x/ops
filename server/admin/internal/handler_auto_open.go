package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/autoopen"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerAutoOpenRoutes(r *gin.RouterGroup) {
	if a.autoOpen == nil {
		return
	}
	r.GET("/auto-open-policies", requirePermission(model.PermissionServicesView), a.listAutoOpenPolicies)
	r.GET("/auto-open-templates", requireAnyPermission(model.PermissionServicesAutoCreate, model.PermissionServicesAutoEdit), a.listAutoOpenTemplates)
	r.POST("/auto-open-policies", requirePermission(model.PermissionServicesAutoCreate), a.saveAutoOpenPolicy)
	r.PUT("/auto-open-policies/:id", requirePermission(model.PermissionServicesAutoEdit), a.saveAutoOpenPolicy)
	r.PUT("/auto-open-policies/:id/enabled", requireAnyPermission(model.PermissionServicesAutoEnable, model.PermissionServicesAutoDisable), a.setAutoOpenEnabled)
}

func (a *AdminService) listAutoOpenPolicies(c *gin.Context) {
	items, err := a.autoOpen.List(c.Request.Context(), currentUser(c))
	if err != nil {
		writeAutoOpenError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) listAutoOpenTemplates(c *gin.Context) {
	items, err := a.autoOpen.Templates(c.Request.Context(), currentUser(c))
	if err != nil {
		writeAutoOpenError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) saveAutoOpenPolicy(c *gin.Context) {
	var request struct {
		model.AutoOpenSettings
		Revision int64 `json:"revision"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAutoOpenError(c, autoopen.ErrInvalid)
		return
	}
	id := c.Param("id")
	item, err := a.autoOpen.Save(c.Request.Context(), currentUser(c), id, request.Revision, request.AutoOpenSettings)
	if err != nil {
		writeAutoOpenError(c, err)
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"ok": true, "data": item})
}

func (a *AdminService) setAutoOpenEnabled(c *gin.Context) {
	var request struct {
		Enabled  *bool `json:"enabled"`
		Revision int64 `json:"revision"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		writeAutoOpenError(c, autoopen.ErrInvalid)
		return
	}
	item, err := a.autoOpen.SetEnabled(c.Request.Context(), currentUser(c), c.Param("id"), request.Revision, *request.Enabled)
	if err != nil {
		writeAutoOpenError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": item})
}

func writeAutoOpenError(c *gin.Context, err error) {
	status := http.StatusServiceUnavailable
	message := "自动开服服务暂不可用"
	switch {
	case errors.Is(err, model.ErrNotFound):
		status, message = http.StatusNotFound, "策略或 Game 模板不存在"
	case errors.Is(err, autoopen.ErrForbidden):
		status, message = http.StatusForbidden, err.Error()
	case errors.Is(err, autoopen.ErrInvalid):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, autoopen.ErrChanged):
		status, message = http.StatusConflict, err.Error()
	}
	c.JSON(status, gin.H{"ok": false, "error": message})
}
