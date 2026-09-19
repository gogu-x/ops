package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/internal/model"
)

func (a *Actor) registerHostRoutes(r *gin.RouterGroup) {
	r.GET("/hosts", a.listHosts)
	r.POST("/hosts", requireRole("admin"), a.createHost)
	r.DELETE("/hosts/:id", requireRole("admin"), a.deleteHost)
	r.POST("/hosts/:id/test", requireRole("admin"), a.testHost)
}

func (a *Actor) listHosts(c *gin.Context) {
	items, err := a.app.ListHosts()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *Actor) createHost(c *gin.Context) {
	var request model.Host
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid host request"})
		return
	}
	created, err := a.app.CreateHost(request)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, host.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *Actor) deleteHost(c *gin.Context) {
	err := a.app.DeleteHost(c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, host.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *Actor) testHost(c *gin.Context) {
	result, err := a.app.TestHost(c.Param("id"))
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, host.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}
