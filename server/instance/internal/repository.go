package internal

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/gogu-x/ops/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrNotFound and ErrAlreadyExists alias the shared model sentinels so that
// admin/internal's HTTP handlers (which cannot import this internal
// package) can distinguish error cases via model.ErrNotFound /
// model.ErrAlreadyExists, while existing call sites within this package
// keep using the unqualified names.
var ErrNotFound = model.ErrNotFound
var ErrAlreadyExists = model.ErrAlreadyExists

type Repository interface {
	List(ctx context.Context) ([]model.ServiceInstance, error)
	Get(ctx context.Context, id string) (model.ServiceInstance, error)
	Create(ctx context.Context, item model.ServiceInstance) error
	Update(ctx context.Context, item model.ServiceInstance) error
	Delete(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]model.ServiceInstance
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.ServiceInstance)}
}

func (r *MemoryRepository) List(context.Context) ([]model.ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.ServiceInstance, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return model.ServiceInstance{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, item model.ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.ServiceTypeID == item.ServiceTypeID && existing.Name == item.Name {
			return ErrAlreadyExists
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, item model.ServiceInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range r.items {
		if id != item.ID && existing.ServiceTypeID == item.ServiceTypeID && existing.Name == item.Name {
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

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) *MongoRepository { return &MongoRepository{db: db} }

func (r *MongoRepository) collection() *mongo.Collection { return r.db.Collection("service_instances") }

func (r *MongoRepository) List(ctx context.Context) ([]model.ServiceInstance, error) {
	var result []model.ServiceInstance
	cursor, err := r.collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.ServiceInstance, error) {
	var item model.ServiceInstance
	err := r.collection().FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.ServiceInstance{}, ErrNotFound
		}
		return model.ServiceInstance{}, err
	}
	return item, nil
}

func (r *MongoRepository) Create(ctx context.Context, item model.ServiceInstance) error {
	_, err := r.collection().InsertOne(ctx, item)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Update(ctx context.Context, item model.ServiceInstance) error {
	res, err := r.collection().ReplaceOne(ctx, bson.M{"_id": item.ID}, item)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) Delete(ctx context.Context, id string) error {
	res, err := r.collection().DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// NewServiceInstance constructs a new service instance. It delegates to
// model.NewServiceInstance, kept as a package-level function here for
// backward compatibility with existing call sites/tests.
func NewServiceInstance(serviceTypeID, hostID, name, image, note, envText, network, portMapping string, params []model.ServiceParam) model.ServiceInstance {
	return model.NewServiceInstance(serviceTypeID, hostID, name, image, note, envText, network, portMapping, params)
}
