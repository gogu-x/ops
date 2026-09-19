package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/admin/internal/auth"
	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/admin/internal/store"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// AdminService owns the HTTP server lifecycle. Request handling is split by resource
// into handler_*.go files so this type remains an infrastructure boundary.
type AdminService struct {
	auth *auth.Service
	http *http.Server
	app  *application.Service
}

// NewAdminService wires the admin actor's dependencies: a Mongo-backed auth
// repository, and Mongo-backed project/service-type/service-instance
// repositories used by the application layer. Hosts are reached through
// the actor Gateway (ops-host), since the host actor owns its own Mongo
// repository.
func NewAdminService(mongodbData *mongo.Database) *AdminService {
	authRepo := store.NewMongoRepository(mongodbData)
	app := application.New(
		application.NewTreeGateway(20*time.Second),
		project.NewMongoRepository(mongodbData),
		environment.NewMongoRepository(mongodbData),
		service.NewMongoRepository(mongodbData),
		instance.NewMongoRepository(mongodbData),
	)
	return &AdminService{
		auth: auth.NewService(authRepo),
		app:  app,
	}
}

func (a *AdminService) Name() string { return "ops-http" }

func (a *AdminService) OnInit(_ tree.Context) {

	if err := a.auth.Bootstrap(context.Background()); err != nil {
		panic("bootstrap admin: " + err.Error())
	}
	gin.SetMode(gin.ReleaseMode)
	router := a.newRouter()
	a.http = &http.Server{Addr: fmt.Sprintf("0.0.0.0:%v", conf.Addr), Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := a.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			println("[ops] http server:", err.Error())
		}
	}()
	println("[ops] HTTP server listening on", conf.Addr)
}

func (a *AdminService) HandleMessage(_ tree.Context, _ interface{}) {}

func (a *AdminService) OnStop(_ tree.Context) {
	if a.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.http.Shutdown(ctx)
		cancel()
	}
}
