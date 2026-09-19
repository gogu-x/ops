package project

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/ops/ops/mongorpc"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrNotFound = errors.New("project not found")
var ErrAlreadyExists = errors.New("project already exists")

type Repository interface {
	List(ctx context.Context) ([]model.Project, error)
	Get(ctx context.Context, id string) (model.Project, error)
	Create(ctx context.Context, item model.Project) error
	Update(ctx context.Context, item model.Project) error
	Delete(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]model.Project
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.Project)}
}

func (r *MemoryRepository) List(context.Context) ([]model.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Project, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return model.Project{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, item model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.Name == item.Name {
			return ErrAlreadyExists
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, item model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range r.items {
		if id != item.ID && existing.Name == item.Name {
			return ErrAlreadyExists
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type MongoRepository struct{ actor string }

func NewMongoRepository(actor string) *MongoRepository { return &MongoRepository{actor: actor} }

func (r *MongoRepository) List(context.Context) ([]model.Project, error) {
	var result []model.Project
	_, err := mongorpc.Request(r.actor, &mongorpc.FindMany{Collection: "projects", Filter: bson.M{}, Results: &result})
	return result, err
}
func (r *MongoRepository) Get(_ context.Context, id string) (model.Project, error) {
	var item model.Project
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOne{Collection: "projects", Filter: bson.M{"_id": id}, Result: &item})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.Project{}, ErrNotFound
	}
	return item, err
}
func (r *MongoRepository) Create(_ context.Context, item model.Project) error {
	_, err := mongorpc.Request(r.actor, &mongorpc.InsertOne{Collection: "projects", Doc: item})
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrAlreadyExists
	}
	return err
}
func (r *MongoRepository) Update(_ context.Context, item model.Project) error {
	value, err := mongorpc.Request(r.actor, &mongorpc.ReplaceOne{Collection: "projects", Filter: bson.M{"_id": item.ID}, Replacement: item})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyExists
		}
		return err
	}
	if value.(mongorpc.WriteResult).MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *MongoRepository) Delete(_ context.Context, id string) error {
	value, err := mongorpc.Request(r.actor, &mongorpc.DeleteOne{Collection: "projects", Filter: bson.M{"_id": id}})
	if err != nil {
		return err
	}
	if value.(mongorpc.WriteResult).DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func NewProject(name, note, envVars string) model.Project {
	now := time.Now().UTC()
	return model.Project{ID: uuid.NewString(), Name: name, Note: note, EnvVars: envVars, CreatedAt: now, UpdatedAt: now}
}
