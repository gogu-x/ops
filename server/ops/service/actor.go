package service

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
type CreateRequest struct{ ServiceType model.ServiceType }
type UpdateRequest struct{ ServiceType model.ServiceType }
type DeleteRequest struct{ ID string }
type ListResponse struct{ ServiceTypes []model.ServiceType }

type Actor struct {
	cfg  conf.Config
	repo Repository
}

func NewActor(cfg conf.Config) *Actor {
	return &Actor{cfg: cfg, repo: NewMemoryRepository()}
}

func (a *Actor) Name() string { return "ops-service" }

func (a *Actor) OnInit(_ tree.Context) {
	if a.cfg.MongoURI != "" {
		repo, err := NewMongoRepository(context.Background(), a.cfg)
		if err != nil {
			panic("connect service type repository: " + err.Error())
		}
		a.repo = repo
	}
}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case ListRequest:
		items, err := a.repo.List(context.Background())
		ctx.Response(ListResponse{ServiceTypes: items}, err)
	case CreateRequest:
		created, err := a.create(request.ServiceType)
		ctx.Response(created, err)
	case UpdateRequest:
		updated, err := a.update(request.ServiceType)
		ctx.Response(updated, err)
	case DeleteRequest:
		ctx.Response(nil, a.repo.Delete(context.Background(), request.ID))
	default:
		ctx.Response(nil, fmt.Errorf("unsupported service type message %T", message))
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.repo.Close(ctx)
}

func (a *Actor) create(item model.ServiceType) (model.ServiceType, error) {
	if err := validate(item); err != nil {
		return model.ServiceType{}, err
	}
	created := NewServiceType(item.HostID, item.Name, item.Image, item.Note, item.Params)
	if err := a.repo.Create(context.Background(), created); err != nil {
		return model.ServiceType{}, err
	}
	return created, nil
}

func (a *Actor) update(item model.ServiceType) (model.ServiceType, error) {
	if err := validate(item); err != nil {
		return model.ServiceType{}, err
	}
	existing, err := a.repo.Get(context.Background(), item.ID)
	if err != nil {
		return model.ServiceType{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := a.repo.Update(context.Background(), item); err != nil {
		return model.ServiceType{}, err
	}
	return item, nil
}

var serviceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func validate(item model.ServiceType) error {
	if strings.TrimSpace(item.HostID) == "" {
		return errors.New("host_id is required")
	}
	if !serviceNamePattern.MatchString(item.Name) {
		return errors.New("name must be lowercase letters, numbers, _ or - and start with a letter")
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
	return nil
}
