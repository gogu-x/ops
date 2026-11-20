package autoopen

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrChanged = errors.New("策略状态已变更，请刷新后重试")

type Repository interface {
	EnsureIndexes(context.Context) error
	List(context.Context) ([]model.AutoOpenPolicy, error)
	Get(context.Context, string) (model.AutoOpenPolicy, error)
	Create(context.Context, model.AutoOpenPolicy) error
	Replace(context.Context, model.AutoOpenPolicy) error
	Due(context.Context, time.Time) (model.AutoOpenPolicy, error)
}

type MemoryRepository struct {
	mu    sync.Mutex
	items map[string]model.AutoOpenPolicy
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.AutoOpenPolicy)}
}

func (r *MemoryRepository) EnsureIndexes(context.Context) error { return nil }

func (r *MemoryRepository) List(context.Context) ([]model.AutoOpenPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]model.AutoOpenPolicy, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.AutoOpenPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, found := r.items[id]
	if !found {
		return model.AutoOpenPolicy{}, model.ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, item model.AutoOpenPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.ID]; exists {
		return model.ErrAlreadyExists
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Replace(_ context.Context, item model.AutoOpenPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.items[item.ID]
	if !found || item.Revision != existing.Revision+1 {
		return ErrChanged
	}
	r.items[item.ID] = item
	return nil
}

func (r *MemoryRepository) Due(_ context.Context, now time.Time) (model.AutoOpenPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due model.AutoOpenPolicy
	for _, item := range r.items {
		if item.Enabled && !item.NextOpenAt.After(now) && !item.LeaseUntil.After(now) &&
			(due.ID == "" || item.NextOpenAt.Before(due.NextOpenAt)) {
			due = item
		}
	}
	if due.ID == "" {
		return due, model.ErrNotFound
	}
	return due, nil
}

type MongoRepository struct {
	items *mongo.Collection
}

func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{items: db.Collection("auto_open_policies")}
}

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.items.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "enabled", Value: 1}, {Key: "next_open_at", Value: 1}},
	})
	return err
}

func (r *MongoRepository) List(ctx context.Context) ([]model.AutoOpenPolicy, error) {
	cursor, err := r.items.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	items := make([]model.AutoOpenPolicy, 0)
	err = cursor.All(ctx, &items)
	return items, err
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.AutoOpenPolicy, error) {
	var item model.AutoOpenPolicy
	if err := r.items.FindOne(ctx, bson.M{"_id": id}).Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = model.ErrNotFound
		}
		return item, err
	}
	return item, nil
}

func (r *MongoRepository) Create(ctx context.Context, item model.AutoOpenPolicy) error {
	_, err := r.items.InsertOne(ctx, item)
	return err
}

func (r *MongoRepository) Replace(ctx context.Context, item model.AutoOpenPolicy) error {
	result, err := r.items.ReplaceOne(ctx, bson.M{"_id": item.ID, "revision": item.Revision - 1}, item)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrChanged
	}
	return nil
}

func (r *MongoRepository) Due(ctx context.Context, now time.Time) (model.AutoOpenPolicy, error) {
	var item model.AutoOpenPolicy
	err := r.items.FindOne(ctx, bson.M{
		"enabled": true, "next_open_at": bson.M{"$lte": now}, "lease_until": bson.M{"$lte": now},
	}, options.FindOne().SetSort(bson.D{{Key: "next_open_at", Value: 1}})).Decode(&item)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = model.ErrNotFound
	}
	return item, err
}
