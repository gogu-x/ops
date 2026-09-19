package service

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

var ErrNotFound = errors.New("service type not found")
var ErrAlreadyExists = errors.New("service type already exists")

type Repository interface {
	List(ctx context.Context) ([]model.ServiceType, error)
	Get(ctx context.Context, id string) (model.ServiceType, error)
	Create(ctx context.Context, item model.ServiceType) error
	Update(ctx context.Context, item model.ServiceType) error
	Delete(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]model.ServiceType
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.ServiceType)}
}

func (r *MemoryRepository) List(context.Context) ([]model.ServiceType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.ServiceType, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.ServiceType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return model.ServiceType{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, item model.ServiceType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.ProjectID == item.ProjectID && existing.HostID == item.HostID && existing.Name == item.Name {
			return ErrAlreadyExists
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, item model.ServiceType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range r.items {
		if id != item.ID && existing.ProjectID == item.ProjectID && existing.HostID == item.HostID && existing.Name == item.Name {
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

func (r *MongoRepository) List(context.Context) ([]model.ServiceType, error) {
	var result []model.ServiceType
	_, err := mongorpc.Request(r.actor, &mongorpc.FindMany{Collection: "service_types", Filter: bson.M{}, Results: &result})
	return result, err
}

func (r *MongoRepository) Get(_ context.Context, id string) (model.ServiceType, error) {
	var item model.ServiceType
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOne{Collection: "service_types", Filter: bson.M{"_id": id}, Result: &item})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.ServiceType{}, ErrNotFound
	}
	return item, err
}

func (r *MongoRepository) Create(_ context.Context, item model.ServiceType) error {
	_, err := mongorpc.Request(r.actor, &mongorpc.InsertOne{Collection: "service_types", Doc: item})
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Update(_ context.Context, item model.ServiceType) error {
	v, err := mongorpc.Request(r.actor, &mongorpc.ReplaceOne{Collection: "service_types", Filter: bson.M{"_id": item.ID}, Replacement: item})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyExists
		}
		return err
	}
	if v.(mongorpc.WriteResult).MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) Delete(_ context.Context, id string) error {
	v, err := mongorpc.Request(r.actor, &mongorpc.DeleteOne{Collection: "service_types", Filter: bson.M{"_id": id}})
	if err != nil {
		return err
	}
	if v.(mongorpc.WriteResult).DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func NewServiceType(projectID, hostID, name string) model.ServiceType {
	now := time.Now().UTC()
	return model.ServiceType{ID: uuid.NewString(), ProjectID: projectID, HostID: hostID, Name: name, CreatedAt: now, UpdatedAt: now}
}
