package internal

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EventRepository stores and retrieves instance lifecycle events.
type EventRepository interface {
	Create(ctx context.Context, event model.InstanceEvent) error
	ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.InstanceEvent, error)
}

// NewEvent constructs a new InstanceEvent with a generated ID and creation
// timestamp.
func NewEvent(instanceID, eventType, message string) model.InstanceEvent {
	return model.InstanceEvent{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		Type:       eventType,
		Message:    message,
		CreatedAt:  time.Now().UTC(),
	}
}

// MemoryEventRepository is an in-memory implementation of EventRepository,
// useful for tests or when no database is configured.
type MemoryEventRepository struct {
	mu    sync.RWMutex
	items []model.InstanceEvent
}

func NewMemoryEventRepository() *MemoryEventRepository {
	return &MemoryEventRepository{}
}

func (r *MemoryEventRepository) Create(_ context.Context, event model.InstanceEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, event)
	return nil
}

func (r *MemoryEventRepository) ListByInstance(_ context.Context, instanceID string, limit int) ([]model.InstanceEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []model.InstanceEvent
	for _, item := range r.items {
		if item.InstanceID == instanceID {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// MongoEventRepository is a Mongo-backed implementation of EventRepository.
type MongoEventRepository struct{ db *mongo.Database }

func NewMongoEventRepository(db *mongo.Database) *MongoEventRepository {
	return &MongoEventRepository{db: db}
}

func (r *MongoEventRepository) collection() *mongo.Collection {
	return r.db.Collection("instance_events")
}

func (r *MongoEventRepository) Create(ctx context.Context, event model.InstanceEvent) error {
	_, err := r.collection().InsertOne(ctx, event)
	return err
}

func (r *MongoEventRepository) ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.InstanceEvent, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := r.collection().Find(ctx, bson.M{"instance_id": instanceID}, opts)
	if err != nil {
		return nil, err
	}
	var result []model.InstanceEvent
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}
