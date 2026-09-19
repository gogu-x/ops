package internal

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/application"
	"github.com/gogu-x/ops/ops/internal/auth"
	"github.com/gogu-x/ops/ops/internal/store"
	"github.com/gogu-x/tree"
)

// Actor owns the HTTP server lifecycle. Request handling is split by resource
// into handler_*.go files so this type remains an infrastructure boundary.
type Actor struct {
	cfg     conf.Config
	auth    *auth.Service
	app     *application.Service
	http    *http.Server
	started chan struct{}
}

func NewActor(cfg conf.Config, repo store.Repository, app *application.Service) *Actor {
	return &Actor{
		cfg: cfg, auth: auth.NewService(repo, cfg),
		app: app, started: make(chan struct{}),
	}
}

func (a *Actor) Name() string { return "ops-http" }

func (a *Actor) OnInit(_ tree.Context) {
	if err := a.auth.Bootstrap(context.Background()); err != nil {
		panic("bootstrap admin: " + err.Error())
	}
	gin.SetMode(gin.ReleaseMode)
	router := a.newRouter()
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
		_ = a.http.Shutdown(ctx)
		cancel()
	}
}
