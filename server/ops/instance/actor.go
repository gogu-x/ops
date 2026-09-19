package instance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/internal/service"
	"github.com/gogu-x/tree"
)

type ListResponse struct{ ServiceInstances []model.ServiceInstance }
type EnrichRequest struct{ ServiceInstances []model.ServiceInstance }

type Actor struct {
	repo         Repository
	serviceTypes service.Repository
}

func NewActor(_ conf.Config, repositories ...Repository) *Actor {
	repo := Repository(NewMemoryRepository())
	if len(repositories) > 0 && repositories[0] != nil {
		repo = repositories[0]
	}
	return &Actor{repo: repo}
}

func NewActorWithRepositories(cfg conf.Config, repo Repository, serviceTypes service.Repository) *Actor {
	a := NewActor(cfg, repo)
	a.serviceTypes = serviceTypes
	return a
}

func (a *Actor) Name() string { return "ops-instance" }

func (a *Actor) OnInit(_ tree.Context) {}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case EnrichRequest:
		ctx.Response(ListResponse{ServiceInstances: a.enrichList(request.ServiceInstances)}, nil)
	case DeployRequest:
		result, err := a.deploy(request.ID)
		ctx.Response(result, err)
	case UpdateImageRequest:
		result, err := a.updateImage(request.ID, request.Image)
		ctx.Response(result, err)
	case ContainerActionRequest:
		err := a.containerAction(request.ID, request.Action)
		ctx.Response(nil, err)
	case RemoveContainerRequest:
		err := a.removeContainer(request.ID)
		ctx.Response(nil, err)
	case StatusRequest:
		result, err := a.status(request.ID)
		ctx.Response(result, err)
	case DetailRequest:
		result, err := a.detail(request.ID)
		ctx.Response(DetailResponse{Detail: result}, err)
	case LogsRequest:
		logs, err := a.logs(request.ID, request.Tail)
		ctx.Response(LogsResponse{Logs: logs}, err)
	default:
		ctx.Response(nil, fmt.Errorf("unsupported service instance message %T", message))
	}
}

func (a *Actor) OnStop(_ tree.Context) {}

func bgCtx() context.Context { return context.Background() }

var instanceNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func Validate(item model.ServiceInstance) error {
	if strings.TrimSpace(item.ServiceTypeID) == "" {
		return errors.New("service_type_id is required")
	}
	if !instanceNamePattern.MatchString(item.Name) {
		return errors.New("name must contain only lowercase letters, numbers, _ or -")
	}
	if strings.TrimSpace(item.Image) == "" {
		return errors.New("image is required")
	}
	for index, param := range item.Params {
		if !strings.HasPrefix(strings.TrimSpace(param.Flag), "--") {
			return fmt.Errorf("params[%d].flag must start with --", index)
		}
		if strings.TrimSpace(param.Value) == "" {
			return fmt.Errorf("params[%d].value is required", index)
		}
	}
	if strings.TrimSpace(item.PortMapping) != "" {
		if _, ok := parsePortMappingField(item.PortMapping); !ok {
			return errors.New("port_mapping must look like host_port:container_port, e.g. 9001:9001")
		}
	}
	return nil
}
