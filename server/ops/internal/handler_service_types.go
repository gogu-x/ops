package internal

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/service"
)

func (a *Actor) registerServiceTypeRoutes(r *gin.RouterGroup) {
	r.GET("/service-types", a.listServiceTypes)
	r.POST("/service-types", requireRole("admin"), a.createServiceType)
	r.PUT("/service-types/:id", requireRole("admin"), a.updateServiceType)
	r.DELETE("/service-types/:id", requireRole("admin"), a.deleteServiceType)
}

func (a *Actor) listServiceTypes(c *gin.Context) {
	items, err := a.app.ListServiceTypes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": items})
}

func (a *Actor) createServiceType(c *gin.Context) {
	var item model.ServiceType
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service type request"})
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

func (a *Actor) updateServiceType(c *gin.Context) {
	var item model.ServiceType
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service type request"})
		return
	}
	item.ID = c.Param("id")
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

func (a *Actor) deleteServiceType(c *gin.Context) {
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
