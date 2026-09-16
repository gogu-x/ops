package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("service type not found")
var ErrAlreadyExists = errors.New("service type already exists")

type Repository interface {
	List(ctx context.Context) ([]model.ServiceType, error)
	Get(ctx context.Context, id string) (model.ServiceType, error)
	Create(ctx context.Context, item model.ServiceType) error
	Update(ctx context.Context, item model.ServiceType) error
	Delete(ctx context.Context, id string) error
	Close(ctx context.Context) error
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
		if existing.Name == item.Name {
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

func (r *MemoryRepository) Close(context.Context) error { return nil }

type MongoRepository struct {
	client *mongo.Client
	items  *mongo.Collection
}

func NewMongoRepository(ctx context.Context, cfg conf.Config) (*MongoRepository, error) {
	clientOptions := options.Client().ApplyURI(cfg.MongoURI)
	if cfg.MongoUsername != "" {
		clientOptions.SetAuth(options.Credential{Username: cfg.MongoUsername, Password: cfg.MongoPassword})
	}
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return &MongoRepository{client: client, items: client.Database(cfg.MongoDatabase).Collection("service_types")}, nil
}

func (r *MongoRepository) List(ctx context.Context) ([]model.ServiceType, error) {
	cursor, err := r.items.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var result []model.ServiceType
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.ServiceType, error) {
	var item model.ServiceType
	if err := r.items.FindOne(ctx, bson.M{"_id": id}).Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.ServiceType{}, ErrNotFound
		}
		return model.ServiceType{}, err
	}
	return item, nil
}

func (r *MongoRepository) Create(ctx context.Context, item model.ServiceType) error {
	_, err := r.items.InsertOne(ctx, item)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Update(ctx context.Context, item model.ServiceType) error {
	result, err := r.items.ReplaceOne(ctx, bson.M{"_id": item.ID}, item)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.items.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) Close(ctx context.Context) error { return r.client.Disconnect(ctx) }

func NewServiceType(hostID, name, image, note string, params []model.ServiceParam) model.ServiceType {
	now := time.Now().UTC()
	return model.ServiceType{ID: uuid.NewString(), HostID: hostID, Name: name, Image: image, Note: note, Params: params, CreatedAt: now, UpdatedAt: now}
}
