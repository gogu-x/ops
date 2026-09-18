package instance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/tree"
)

type ListRequest struct{}
type CreateRequest struct{ ServiceInstance model.ServiceInstance }
type UpdateRequest struct{ ServiceInstance model.ServiceInstance }
type DeleteRequest struct{ ID string }
type ListResponse struct{ ServiceInstances []model.ServiceInstance }

type Actor struct {
	cfg  conf.Config
	repo Repository
}

func NewActor(cfg conf.Config) *Actor {
	return &Actor{cfg: cfg, repo: NewMemoryRepository()}
}

func (a *Actor) Name() string { return "ops-instance" }

func (a *Actor) OnInit(_ tree.Context) {
	if a.cfg.MongoURI != "" {
		repo, err := NewMongoRepository(context.Background(), a.cfg)
		if err != nil {
			panic("connect service instance repository: " + err.Error())
		}
		a.repo = repo
	}
}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case ListRequest:
		items, err := a.repo.List(context.Background())
		if err == nil {
			items = enrichList(items)
		}
		ctx.Response(ListResponse{ServiceInstances: items}, err)
	case CreateRequest:
		created, err := a.create(request.ServiceInstance)
		ctx.Response(created, err)
	case UpdateRequest:
		updated, err := a.update(request.ServiceInstance)
		ctx.Response(updated, err)
	case DeleteRequest:
		ctx.Response(nil, a.delete(request.ID))
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

func (a *Actor) OnStop(_ tree.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.repo.Close(ctx)
}

func bgCtx() context.Context { return context.Background() }

func (a *Actor) delete(id string) error {
	// Best-effort remove the backing container first; ignore errors caused by
	// the instance never having been deployed or the host being unavailable,
	// so metadata deletion is not blocked by container cleanup failures.
	_ = a.removeContainer(id)
	return a.repo.Delete(context.Background(), id)
}

func (a *Actor) create(item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := validate(item); err != nil {
		return model.ServiceInstance{}, err
	}
	created := NewServiceInstance(item.ServiceTypeID, item.Name, item.Image, item.Note, item.EnvText, item.Network, item.PortMapping, item.Params)
	if err := a.repo.Create(context.Background(), created); err != nil {
		return model.ServiceInstance{}, err
	}
	return created, nil
}

func (a *Actor) update(item model.ServiceInstance) (model.ServiceInstance, error) {
	if err := validate(item); err != nil {
		return model.ServiceInstance{}, err
	}
	existing, err := a.repo.Get(context.Background(), item.ID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := a.repo.Update(context.Background(), item); err != nil {
		return model.ServiceInstance{}, err
	}
	return item, nil
}

var instanceNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validate(item model.ServiceInstance) error {
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
