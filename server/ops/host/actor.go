package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/tree"
)

type ListRequest struct{}
type CreateRequest struct{ Host model.Host }
type DeleteRequest struct{ ID string }
type TestRequest struct{ ID string }

type ListResponse struct{ Hosts []model.Host }
type TestResponse struct {
	Host   model.Host        `json:"host"`
	Docker map[string]string `json:"docker"`
}

type Actor struct {
	cfg    conf.Config
	repo   Repository
	docker *DockerManager
}

func NewActor(cfg conf.Config) *Actor {
	return &Actor{cfg: cfg, repo: NewMemoryRepository(), docker: NewDockerManager()}
}

func (a *Actor) Name() string { return "ops-host" }

func (a *Actor) OnInit(_ tree.Context) {
	if a.cfg.MongoURI != "" {
		repo, err := NewMongoRepository(context.Background(), a.cfg)
		if err != nil {
			panic("connect host repository: " + err.Error())
		}
		a.repo = repo
	}
}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case ListRequest:
		hosts, err := a.repo.List(context.Background())
		for i := range hosts {
			hosts[i] = publicHost(hosts[i])
		}
		ctx.Response(ListResponse{Hosts: hosts}, err)
	case CreateRequest:
		created, err := a.create(request.Host)
		ctx.Response(publicHost(created), err)
	case DeleteRequest:
		err := a.delete(request.ID)
		ctx.Response(nil, err)
	case TestRequest:
		result, err := a.test(request.ID)
		ctx.Response(result, err)
	default:
		ctx.Response(nil, fmt.Errorf("unsupported host message %T", message))
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	_ = a.docker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.repo.Close(ctx)
}

func (a *Actor) create(host model.Host) (model.Host, error) {
	if host.Name == "" {
		return model.Host{}, errors.New("host name is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(host.DockerHost), "tcp://") {
		return model.Host{}, errors.New("docker_host must use tcp://")
	}
	if !strings.HasSuffix(strings.TrimSpace(host.DockerHost), ":2376") {
		return model.Host{}, errors.New("docker_host must use TCP port 2376")
	}
	if strings.TrimSpace(host.TLSCA) == "" || strings.TrimSpace(host.TLSCert) == "" || strings.TrimSpace(host.TLSKey) == "" {
		// TLS material is optional: Docker TLS remains enabled, while the
		// client skips server certificate verification when CA is empty.
	}
	created := NewHost(host.Name, host.DockerHost, host.TLSCA, host.TLSCert, host.TLSKey, host.Note)
	host.ID = created.ID
	host.CreatedAt = created.CreatedAt
	host.UpdatedAt = created.UpdatedAt
	if err := a.repo.Create(context.Background(), host); err != nil {
		return model.Host{}, err
	}
	return host, nil
}

func (a *Actor) delete(id string) error {
	if _, err := a.repo.Get(context.Background(), id); err != nil {
		return err
	}
	return a.repo.Delete(context.Background(), id)
}

func (a *Actor) test(id string) (TestResponse, error) {
	host, err := a.repo.Get(context.Background(), id)
	if err != nil {
		return TestResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := a.docker.Test(ctx, host)
	if err != nil {
		return TestResponse{}, err
	}
	return TestResponse{Host: publicHost(host), Docker: info}, nil
}

func publicHost(host model.Host) model.Host {
	if host.TLSCA != "" {
		host.TLSCA = "[已配置]"
	}
	if host.TLSCert != "" {
		host.TLSCert = "[已配置]"
	}
	if host.TLSKey != "" {
		host.TLSKey = "[已配置]"
	}
	return host
}
