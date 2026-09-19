package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerEnvironmentRoutes(r *gin.RouterGroup) {
	r.GET("/environments", a.listEnvironments)
	r.POST("/environments", requireRole("admin"), a.createEnvironment)
	r.PUT("/environments/:id", requireRole("admin"), a.updateEnvironment)
	r.DELETE("/environments/:id", requireRole("admin"), a.deleteEnvironment)
}

func (a *AdminService) listEnvironments(c *gin.Context) {
	items, err := a.app.ListEnvironments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createEnvironment(c *gin.Context) {
	var item model.Environment
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid environment request"})
		return
	}
	created, err := a.app.CreateEnvironment(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, environment.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *AdminService) updateEnvironment(c *gin.Context) {
	var item model.Environment
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid environment request"})
		return
	}
	item.ID = c.Param("id")
	updated, err := a.app.UpdateEnvironment(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, environment.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, environment.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *AdminService) deleteEnvironment(c *gin.Context) {
	err := a.app.DeleteEnvironment(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, environment.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
