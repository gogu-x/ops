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
	"github.com/gogu-x/ops/ops/instance"
	"github.com/gogu-x/ops/ops/internal/auth"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/store"
	"github.com/gogu-x/ops/ops/project"
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
	protected.GET("/projects", a.listProjects)
	protected.POST("/projects", requireRole("admin"), a.createProject)
	protected.PUT("/projects/:id", requireRole("admin"), a.updateProject)
	protected.DELETE("/projects/:id", requireRole("admin"), a.deleteProject)
	protected.GET("/service-instances", a.listServiceInstances)
	protected.POST("/service-instances", requireRole("admin"), a.createServiceInstance)
	protected.PUT("/service-instances/:id", requireRole("admin"), a.updateServiceInstance)
	protected.DELETE("/service-instances/:id", requireRole("admin"), a.deleteServiceInstance)
	protected.GET("/service-instances/:id/status", a.serviceInstanceStatus)
	protected.GET("/service-instances/:id/detail", a.serviceInstanceDetail)
	protected.GET("/service-instances/:id/logs", a.serviceInstanceLogs)
	protected.POST("/service-instances/:id/deploy", requireRole("admin"), a.deployServiceInstance)
	protected.POST("/service-instances/:id/update-image", requireRole("admin"), a.updateServiceInstanceImage)
	protected.POST("/service-instances/:id/start", requireRole("admin"), a.startServiceInstance)
	protected.POST("/service-instances/:id/stop", requireRole("admin"), a.stopServiceInstance)
	protected.POST("/service-instances/:id/restart", requireRole("admin"), a.restartServiceInstance)
	protected.DELETE("/service-instances/:id/container", requireRole("admin"), a.removeServiceInstanceContainer)
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
	if err := a.ensureProject(item.ProjectID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
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
	if err := a.ensureProject(item.ProjectID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
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

func projectRequest(message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup("ops-project")
	if !ok {
		return nil, errors.New("project actor is unavailable")
	}
	return tree.Request(pid, message).AwaitTimeout(20 * time.Second)
}

func (a *Actor) ensureProject(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("project_id is required")
	}
	value, err := projectRequest(project.ListRequest{})
	if err != nil {
		return err
	}
	result, ok := value.(project.ListResponse)
	if !ok {
		return errors.New("invalid project actor response")
	}
	for _, item := range result.Projects {
		if item.ID == id {
			return nil
		}
	}
	return errors.New("project not found: " + id)
}

func (a *Actor) listProjects(c *gin.Context) {
	value, err := projectRequest(project.ListRequest{})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(project.ListResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid project actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.Projects})
}

func (a *Actor) createProject(c *gin.Context) {
	var item model.Project
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid project request"})
		return
	}
	value, err := projectRequest(project.CreateRequest{Project: item})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, project.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	created, ok := value.(model.Project)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid project actor response"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *Actor) updateProject(c *gin.Context) {
	var item model.Project
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid project request"})
		return
	}
	item.ID = c.Param("id")
	value, err := projectRequest(project.UpdateRequest{Project: item})
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
	updated, ok := value.(model.Project)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid project actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *Actor) deleteProject(c *gin.Context) {
	_, err := projectRequest(project.DeleteRequest{ID: c.Param("id")})
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

func serviceInstanceRequest(message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup("ops-instance")
	if !ok {
		return nil, errors.New("service instance actor is unavailable")
	}
	return tree.Request(pid, message).AwaitTimeout(20 * time.Second)
}

func (a *Actor) ensureServiceType(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("service_type_id is required")
	}
	value, err := serviceRequest(service.ListRequest{})
	if err != nil {
		return err
	}
	result, ok := value.(service.ListResponse)
	if !ok {
		return errors.New("invalid service actor response")
	}
	for _, item := range result.ServiceTypes {
		if item.ID == id {
			return nil
		}
	}
	return errors.New("service type not found: " + id)
}

func (a *Actor) listServiceInstances(c *gin.Context) {
	value, err := serviceInstanceRequest(instance.ListRequest{})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(instance.ListResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.ServiceInstances})
}

func (a *Actor) createServiceInstance(c *gin.Context) {
	var item model.ServiceInstance
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service instance request"})
		return
	}
	if err := a.ensureServiceType(item.ServiceTypeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	value, err := serviceInstanceRequest(instance.CreateRequest{ServiceInstance: item})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, instance.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	created, ok := value.(model.ServiceInstance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": created})
}

func (a *Actor) updateServiceInstance(c *gin.Context) {
	var item model.ServiceInstance
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid service instance request"})
		return
	}
	item.ID = c.Param("id")
	if err := a.ensureServiceType(item.ServiceTypeID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": err.Error()})
		return
	}
	value, err := serviceInstanceRequest(instance.UpdateRequest{ServiceInstance: item})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, instance.ErrAlreadyExists) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	updated, ok := value.(model.ServiceInstance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": updated})
}

func (a *Actor) deleteServiceInstance(c *gin.Context) {
	_, err := serviceInstanceRequest(instance.DeleteRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *Actor) deployServiceInstance(c *gin.Context) {
	value, err := serviceInstanceRequest(instance.DeployRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, instance.ErrHostUnresolved) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(model.ServiceInstance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *Actor) updateServiceInstanceImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request: image is required"})
		return
	}
	value, err := serviceInstanceRequest(instance.UpdateImageRequest{ID: c.Param("id"), Image: req.Image})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, instance.ErrHostUnresolved) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(model.ServiceInstance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *Actor) startServiceInstance(c *gin.Context) {
	a.instanceContainerAction(c, "start")
}

func (a *Actor) stopServiceInstance(c *gin.Context) {
	a.instanceContainerAction(c, "stop")
}

func (a *Actor) restartServiceInstance(c *gin.Context) {
	a.instanceContainerAction(c, "restart")
}

func (a *Actor) instanceContainerAction(c *gin.Context, action string) {
	_, err := serviceInstanceRequest(instance.ContainerActionRequest{ID: c.Param("id"), Action: action})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *Actor) removeServiceInstanceContainer(c *gin.Context) {
	_, err := serviceInstanceRequest(instance.RemoveContainerRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *Actor) serviceInstanceStatus(c *gin.Context) {
	value, err := serviceInstanceRequest(instance.StatusRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(model.ServiceInstance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result})
}

func (a *Actor) serviceInstanceDetail(c *gin.Context) {
	value, err := serviceInstanceRequest(instance.DetailRequest{ID: c.Param("id")})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(instance.DetailResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.Detail})
}

func (a *Actor) serviceInstanceLogs(c *gin.Context) {
	tail := c.DefaultQuery("tail", "200")
	value, err := serviceInstanceRequest(instance.LogsRequest{ID: c.Param("id"), Tail: tail})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, instance.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"ok": false, "error": err.Error()})
		return
	}
	result, ok := value.(instance.LogsResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "invalid service instance actor response"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": result.Logs})
}
