package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/rollout"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerRolloutTemplateRoutes(r *gin.RouterGroup) {
	r.GET("/rollout-templates", requirePermission(model.PermissionServicesView), a.listRolloutTemplates)
	r.POST("/rollout-templates", requirePermission(model.PermissionServicesManage), requirePermission(model.PermissionRolloutTemplatesManage), a.createRolloutTemplate)
}

func (a *AdminService) listRolloutTemplates(c *gin.Context) {
	items, err := a.rollouts.ListTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "读取策略模板失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createRolloutTemplate(c *gin.Context) {
	var item model.RolloutTemplate
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "策略模板格式无效"})
		return
	}
	created, err := a.rollouts.CreateTemplate(c.Request.Context(), item)
	if err != nil {
		status := http.StatusServiceUnavailable
		message := "保存策略模板失败"
		if errors.Is(err, rollout.ErrInvalidTemplate) {
			status = http.StatusBadRequest
			message = err.Error()
		}
		c.JSON(status, gin.H{"ok": false, "error": message})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}
