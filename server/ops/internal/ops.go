package internal

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/internal/auth"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/store"
	"github.com/gogu-x/ops/ops/service"
	"github.com/gogu-x/tree"
)

type Actor struct {
	cfg     conf.Config
	repo    store.Repository
	auth    *auth.Service
	http    *http.Server
	started chan struct{}
}

func NewActor(cfg conf.Config) *Actor {
	var repo store.Repository = store.NewMemoryRepository()
	if cfg.MongoURI != "" {
		mongoRepo, err := store.NewMongoRepository(context.Background(), cfg)
		if err != nil {
			return nil
		}
		repo = mongoRepo
	} else {
		log.Println("[ops] OPS_MONGO_URI 未设置，使用内存仓储；生产环境请配置 MongoDB")
	}
	return &Actor{cfg: cfg, repo: repo, auth: auth.NewService(repo, cfg), started: make(chan struct{})}
}

func (a *Actor) Name() string { return "ops-http" }

func (a *Actor) OnInit(_ tree.Context) {
	if err := a.auth.Bootstrap(context.Background()); err != nil {
		panic("bootstrap admin: " + err.Error())
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware())
	a.registerRoutes(router)
	a.http = &http.Server{Addr: a.cfg.Addr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	close(a.started)
	go func() {
		if err := a.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			println("[ops] http server:", err.Error())
		}
	}()
	println("[ops] HTTP server listening on", a.cfg.Addr)
}

func (a *Actor) HandleMessage(_ tree.Context, _ interface{}) {}

func (a *Actor) OnStop(_ tree.Context) {
	if a.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.http.Shutdown(ctx)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.repo.Close(ctx)
}

func (a *Actor) registerRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "ops"}) })
	api := r.Group("/api")
	authRoutes := api.Group("/auth")
	authRoutes.POST("/login", a.login)
	authRoutes.POST("/refresh", a.refresh)
	authRoutes.POST("/logout", a.logout)
	protected := api.Group("")
	protected.Use(a.authMiddleware())
	protected.GET("/me", a.me)
	protected.GET("/hosts", a.listHosts)
	protected.POST("/hosts", requireRole("admin"), a.createHost)
	protected.DELETE("/hosts/:id", requireRole("admin"), a.deleteHost)
	protected.POST("/hosts/:id/test", requireRole("admin"), a.testHost)
	protected.GET("/service-types", a.listServiceTypes)
	protected.POST("/service-types", requireRole("admin"), a.createServiceType)
	protected.PUT("/service-types/:id", requireRole("admin"), a.updateServiceType)
	protected.DELETE("/service-types/:id", requireRole("admin"), a.deleteServiceType)
	protected.GET("/admin/ping", requireRole("admin"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
}

func (a *Actor) login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request"})
		return
	}
	result, err := a.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "用户名或密码错误"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *Actor) refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request"})
		return
	}
	result, err := a.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "refresh token 无效或已过期"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *Actor) logout(c *gin.Context) {
	var req model.RefreshRequest
	_ = c.ShouldBindJSON(&req)
	if err := a.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "退出登录失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *Actor) me(c *gin.Context) {
	claims, _ := c.Get("claims")
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": claims})
}

func (a *Actor) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "未登录"})
			c.Abort()
			return
		}
		claims, err := a.auth.ParseAccessToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "登录已过期"})
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

func requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := c.Get("claims")
		if !ok || claims.(*auth.Claims).Role != role {
			c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "无权限"})
			c.Abort()
			return
		}
		c.Next()
	}
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

func hostRequest(message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup("ops-host")
	if !ok {
		return nil, errors.New("host actor is unavailable")
	}
	return tree.Request(pid, message).AwaitTimeout(20 * time.Second)
}

func (a *Actor) listHosts(c *gin.Context) {
	value, err := hostRequest(host.ListRequest{})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(host.ListResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid host actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.Hosts})
}

func (a *Actor) createHost(c *gin.Context) {
	var request model.Host
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid host request"})
		return
	}
	value, err := hostRequest(host.CreateRequest{Host: request})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, host.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	created, ok := value.(model.Host)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid host actor response"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *Actor) deleteHost(c *gin.Context) {
	_, err := hostRequest(host.DeleteRequest{ID: c.Param("id")})
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
	value, err := hostRequest(host.TestRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, host.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(host.TestResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid host actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func serviceRequest(message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup("ops-service")
	if !ok {
		return nil, errors.New("service actor is unavailable")
	}
	return tree.Request(pid, message).AwaitTimeout(20 * time.Second)
}

func (a *Actor) listServiceTypes(c *gin.Context) {
	value, err := serviceRequest(service.ListRequest{})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(service.ListResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.ServiceTypes})
}

func (a *Actor) createServiceType(c *gin.Context) {
	var item model.ServiceType
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service type request"})
		return
	}
	if err := a.ensureHost(item.HostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	value, err := serviceRequest(service.CreateRequest{ServiceType: item})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	created, ok := value.(model.ServiceType)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service actor response"})
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
	if err := a.ensureHost(item.HostID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	value, err := serviceRequest(service.UpdateRequest{ServiceType: item})
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
	updated, ok := value.(model.ServiceType)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *Actor) deleteServiceType(c *gin.Context) {
	_, err := serviceRequest(service.DeleteRequest{ID: c.Param("id")})
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

func (a *Actor) ensureHost(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("host_id is required")
	}
	value, err := hostRequest(host.ListRequest{})
	if err != nil {
		return err
	}
	result, ok := value.(host.ListResponse)
	if !ok {
		return errors.New("invalid host actor response")
	}
	for _, item := range result.Hosts {
		if item.ID == id {
			return nil
		}
	}
	return errors.New("host not found: " + id)
}
