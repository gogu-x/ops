package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/model"
)

func (a *AdminService) registerHostRoutes(r *gin.RouterGroup) {
	r.GET("/hosts", requireAnyPermission(model.PermissionHostsView, model.PermissionServicesView, model.PermissionProjectsView), a.listHosts)
	r.GET("/hosts/:id/config", requirePermission(model.PermissionHostsManage), a.getHostConfiguration)
	r.POST("/hosts", requirePermission(model.PermissionHostsManage), a.createHost)
	r.PUT("/hosts/:id", requirePermission(model.PermissionHostsManage), a.updateHost)
	r.DELETE("/hosts/:id", requirePermission(model.PermissionHostsManage), a.deleteHost)
	r.POST("/hosts/:id/test", requirePermission(model.PermissionHostsManage), a.testHost)
	r.POST("/hosts/:id/test-connection", requirePermission(model.PermissionHostsManage), a.testHostConnection)
}

func (a *AdminService) getHostConfiguration(c *gin.Context) {
	if !allowHost(c, a, c.Param("id")) {
		return
	}
	host, err := a.app.GetHostConfiguration(c.Param("id"))
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": host})
}

func (a *AdminService) listHosts(c *gin.Context) {
	items, err := a.app.ListHosts()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	user := currentUser(c)
	if user.Role != model.RoleAdmin {
		visible := items[:0]
		for _, item := range items {
			allowedProjectIDs := make([]string, 0, len(item.ProjectIDs)+1)
			for _, projectID := range append(append([]string{}, item.ProjectIDs...), item.ProjectID) {
				if user.HasProject(projectID) {
					allowedProjectIDs = append(allowedProjectIDs, projectID)
				}
			}
			if len(allowedProjectIDs) == 0 {
				continue
			}
			item.ProjectIDs = allowedProjectIDs
			if !user.HasProject(item.ProjectID) {
				item.ProjectID = allowedProjectIDs[0]
			}
			if !user.HasPermission(model.PermissionHostsManage) {
				item.TLSCA, item.TLSCert, item.TLSKey = "", "", ""
			}
			visible = append(visible, item)
		}
		items = visible
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *AdminService) createHost(c *gin.Context) {
	var request model.Host
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid host request"})
		return
	}
	user := currentUser(c)
	if user.Role != model.RoleAdmin {
		projectIDs := append(append([]string{}, request.ProjectIDs...), request.ProjectID)
		allowed := false
		for _, id := range projectIDs {
			if id == "" {
				continue
			}
			allowed = true
			if !user.HasProject(id) {
				c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "主机只能绑定到已授权项目"})
				return
			}
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "主机必须绑定到已授权项目"})
			return
		}
	}
	created, err := a.app.CreateHost(request)
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

func (a *AdminService) updateHost(c *gin.Context) {
	if !allowHost(c, a, c.Param("id")) {
		return
	}
	var request model.Host
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid host request"})
		return
	}
	request.ID = c.Param("id")
	updated, err := a.app.UpdateHost(request)
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

func (a *AdminService) deleteHost(c *gin.Context) {
	if !allowHost(c, a, c.Param("id")) {
		return
	}
	err := a.app.DeleteHost(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, model.ErrHostBound) || errors.Is(err, model.ErrHostInUse) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *AdminService) testHost(c *gin.Context) {
	if !allowHost(c, a, c.Param("id")) {
		return
	}
	result, err := a.app.TestHost(c.Param("id"))
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *AdminService) testHostConnection(c *gin.Context) {
	if !allowHost(c, a, c.Param("id")) {
		return
	}
	var request model.Host
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid host connection request"})
		return
	}
	request.ID = c.Param("id")
	result, err := a.app.TestHostConnection(request)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, model.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}
