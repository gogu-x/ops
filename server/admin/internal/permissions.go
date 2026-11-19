package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/model"
)

func allowProject(c *gin.Context, projectID string) bool {
	user := currentUser(c)
	if user.Role == model.RoleAdmin || user.HasProject(projectID) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权访问该项目"})
	return false
}

func allowEnvironment(c *gin.Context, a *AdminService, id string) bool {
	items, err := a.app.ListEnvironments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return false
	}
	for _, item := range items {
		if item.ID == id {
			return allowProject(c, item.ProjectID)
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "环境不存在"})
	return false
}

func allowInstance(c *gin.Context, a *AdminService, id string) bool {
	item, err := a.app.GetInstance(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "服务实例不存在"})
			return false
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return false
	}
	return allowProject(c, item.ProjectID)
}

func allowHost(c *gin.Context, a *AdminService, id string) bool {
	user := currentUser(c)
	if user.Role == model.RoleAdmin {
		return true
	}
	hosts, err := a.app.ListHosts()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return false
	}
	for _, host := range hosts {
		if host.ID != id {
			continue
		}
		projectIDs := append(append([]string{}, host.ProjectIDs...), host.ProjectID)
		if len(projectIDs) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权访问未绑定项目的主机"})
			return false
		}
		for _, projectID := range projectIDs {
			if projectID != "" && !user.HasProject(projectID) {
				c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权访问该主机"})
				return false
			}
		}
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "主机不存在"})
	return false
}
