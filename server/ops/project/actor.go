package project

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
type CreateRequest struct{ Project model.Project }
type UpdateRequest struct{ Project model.Project }
type DeleteRequest struct{ ID string }
type ListResponse struct{ Projects []model.Project }

type Actor struct {
	cfg  conf.Config
	repo Repository
}

func NewActor(cfg conf.Config) *Actor {
	return &Actor{cfg: cfg, repo: NewMemoryRepository()}
}

func (a *Actor) Name() string { return "ops-project" }

func (a *Actor) OnInit(_ tree.Context) {
	if a.cfg.MongoURI != "" {
		repo, err := NewMongoRepository(context.Background(), a.cfg)
		if err != nil {
			panic("connect project repository: " + err.Error())
		}
		a.repo = repo
	}
}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case ListRequest:
		items, err := a.repo.List(context.Background())
		ctx.Response(ListResponse{Projects: items}, err)
	case CreateRequest:
		created, err := a.create(request.Project)
		ctx.Response(created, err)
	case UpdateRequest:
		updated, err := a.update(request.Project)
		ctx.Response(updated, err)
	case DeleteRequest:
		ctx.Response(nil, a.repo.Delete(context.Background(), request.ID))
	default:
		ctx.Response(nil, fmt.Errorf("unsupported project message %T", message))
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = a.repo.Close(ctx)
}

func (a *Actor) create(item model.Project) (model.Project, error) {
	if err := validate(item); err != nil {
		return model.Project{}, err
	}
	created := NewProject(item.Name, item.Note, item.EnvVars)
	if err := a.repo.Create(context.Background(), created); err != nil {
		return model.Project{}, err
	}
	return created, nil
}

func (a *Actor) update(item model.Project) (model.Project, error) {
	if err := validate(item); err != nil {
		return model.Project{}, err
	}
	existing, err := a.repo.Get(context.Background(), item.ID)
	if err != nil {
		return model.Project{}, err
	}
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	if err := a.repo.Update(context.Background(), item); err != nil {
		return model.Project{}, err
	}
	return item, nil
}

func validate(item model.Project) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}
