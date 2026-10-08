package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/store"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerUserRoutes(r *gin.RouterGroup) {
	users := r.Group("/users", requireRole(model.RoleAdmin))
	users.GET("", a.listUsers)
	users.POST("", a.createUser)
	users.PUT("/:id", a.updateUser)
	users.PUT("/:id/password", a.resetUserPassword)
}

func (a *AdminService) listUsers(c *gin.Context) {
	users, err := a.auth.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": users})
}

func (a *AdminService) createUser(c *gin.Context) {
	var request model.CreateManagedUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid user request"})
		return
	}
	if err := a.validateAssignedProjects(c, request.ProjectIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	user, err := a.auth.CreateManagedUser(c.Request.Context(), model.User{Username: request.Username, Role: request.Role, ProjectIDs: request.ProjectIDs, Permissions: request.Permissions, Disabled: request.Disabled}, request.Password)
	if err != nil {
		writeUserError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": user})
}

func (a *AdminService) updateUser(c *gin.Context) {
	var request model.UpdateManagedUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid user request"})
		return
	}
	if err := a.validateAssignedProjects(c, request.ProjectIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	user, err := a.auth.UpdateManagedUser(c.Request.Context(), model.User{ID: c.Param("id"), Username: request.Username, Role: request.Role, ProjectIDs: request.ProjectIDs, Permissions: request.Permissions, Disabled: request.Disabled})
	if err != nil {
		writeUserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": user})
}

func (a *AdminService) resetUserPassword(c *gin.Context) {
	var request model.ResetManagedUserPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid password request"})
		return
	}
	if err := a.auth.ResetManagedUserPassword(c.Request.Context(), c.Param("id"), request.Password); err != nil {
		writeUserError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) validateAssignedProjects(c *gin.Context, ids []string) error {
	projects, err := a.app.ListProjects(c.Request.Context())
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(projects))
	for _, project := range projects {
		known[project.ID] = true
	}
	for _, id := range ids {
		if !known[id] {
			return errors.New("项目不存在: " + id)
		}
	}
	return nil
}

func writeUserError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrAlreadyExists) {
		status = http.StatusConflict
	}
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"ok": false, "error": err.Error()})
}
