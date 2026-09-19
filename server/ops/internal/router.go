package internal

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *Actor) newRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware())
	a.registerRoutes(router)
	return router
}

func (a *Actor) registerRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ops"}) })
	api := r.Group("/api")
	a.registerAuthRoutes(api)

	protected := api.Group("")
	protected.Use(a.authMiddleware())
	protected.GET("/me", a.me)
	a.registerHostRoutes(protected)
	a.registerServiceTypeRoutes(protected)
	a.registerProjectRoutes(protected)
	a.registerInstanceRoutes(protected)
	protected.GET("/admin/ping", requireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
