package environment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var ErrNotFound = errors.New("environment not found")
var ErrAlreadyExists = errors.New("environment already exists")

type Repository interface {
	List(ctx context.Context) ([]model.Environment, error)
	Get(ctx context.Context, id string) (model.Environment, error)
	Create(ctx context.Context, item model.Environment) error
	Update(ctx context.Context, item model.Environment) error
	Delete(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]model.Environment
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.Environment)}
}

func (r *MemoryRepository) List(context.Context) ([]model.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Environment, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return model.Environment{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, item model.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.ProjectID == item.ProjectID && existing.Name == item.Name {
			return ErrAlreadyExists
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, item model.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	for id, existing := range r.items {
		if id != item.ID && existing.ProjectID == item.ProjectID && existing.Name == item.Name {
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

func (r *MongoRepository) collection() *mongo.Collection { return r.db.Collection("environments") }

func (r *MongoRepository) List(ctx context.Context) ([]model.Environment, error) {
	var result []model.Environment
	cursor, err := r.collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.Environment, error) {
	var item model.Environment
	err := r.collection().FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.Environment{}, ErrNotFound
		}
		return model.Environment{}, err
	}
	return item, nil
}

func (r *MongoRepository) Create(ctx context.Context, item model.Environment) error {
	_, err := r.collection().InsertOne(ctx, item)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Update(ctx context.Context, item model.Environment) error {
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

func NewEnvironment(projectID, name string) model.Environment {
	now := time.Now().UTC()
	return model.Environment{ID: uuid.NewString(), ProjectID: projectID, Name: name, CreatedAt: now, UpdatedAt: now}
}
