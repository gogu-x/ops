package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerProjectRoutes(r *gin.RouterGroup) {
	r.GET("/projects", a.listProjects)
	r.POST("/projects", requireRole("admin"), a.createProject)
	r.PUT("/projects/:id", requireRole("admin"), a.updateProject)
	r.DELETE("/projects/:id", requireRole("admin"), a.deleteProject)
}

func (a *AdminService) listProjects(c *gin.Context) {
	items, err := a.app.ListProjects(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createProject(c *gin.Context) {
	var item model.Project
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid project request"})
		return
	}
	created, err := a.app.CreateProject(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, project.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *AdminService) updateProject(c *gin.Context) {
	var item model.Project
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid project request"})
		return
	}
	item.ID = c.Param("id")
	updated, err := a.app.UpdateProject(c.Request.Context(), item)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, project.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, project.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *AdminService) deleteProject(c *gin.Context) {
	err := a.app.DeleteProject(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, project.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
