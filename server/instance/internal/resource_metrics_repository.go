package internal

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gogu-x/ops/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	resourceHistoryLimit = 120
	resourceRetention    = 7 * 24 * time.Hour
)

type ResourceMetricsRepository interface {
	Create(ctx context.Context, sample model.ContainerResourceSample) error
	ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.ContainerResourceSample, error)
}

type MemoryResourceMetricsRepository struct {
	mu    sync.RWMutex
	items map[string][]model.ContainerResourceSample
}

func NewMemoryResourceMetricsRepository() *MemoryResourceMetricsRepository {
	return &MemoryResourceMetricsRepository{items: make(map[string][]model.ContainerResourceSample)}
}

func (r *MemoryResourceMetricsRepository) Create(_ context.Context, sample model.ContainerResourceSample) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := append(r.items[sample.InstanceID], sample)
	if len(items) > resourceHistoryLimit {
		items = append([]model.ContainerResourceSample(nil), items[len(items)-resourceHistoryLimit:]...)
	}
	r.items[sample.InstanceID] = items
	return nil
}

func (r *MemoryResourceMetricsRepository) ListByInstance(_ context.Context, instanceID string, limit int) ([]model.ContainerResourceSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := append([]model.ContainerResourceSample(nil), r.items[instanceID]...)
	sort.Slice(result, func(i, j int) bool { return result[i].SampledAt.Before(result[j].SampledAt) })
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}

type MongoResourceMetricsRepository struct{ db *mongo.Database }

func NewMongoResourceMetricsRepository(db *mongo.Database) *MongoResourceMetricsRepository {
	return &MongoResourceMetricsRepository{db: db}
}

func (r *MongoResourceMetricsRepository) collection() *mongo.Collection {
	return r.db.Collection("instance_resource_samples")
}

func (r *MongoResourceMetricsRepository) EnsureIndexes(ctx context.Context) error {
	if _, err := r.collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "instance_id", Value: 1}, {Key: "sampled_at", Value: 1}},
		Options: options.Index().SetName("instance_resource_history"),
	}); err != nil {
		return err
	}
	_, err := r.collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "sampled_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(resourceRetention.Seconds())).SetName("resource_sample_retention"),
	})
	return err
}

func (r *MongoResourceMetricsRepository) Create(ctx context.Context, sample model.ContainerResourceSample) error {
	_, err := r.collection().InsertOne(ctx, sample)
	return err
}

func (r *MongoResourceMetricsRepository) ListByInstance(ctx context.Context, instanceID string, limit int) ([]model.ContainerResourceSample, error) {
	opts := options.Find().SetSort(bson.D{{Key: "sampled_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := r.collection().Find(ctx, bson.M{"instance_id": instanceID}, opts)
	if err != nil {
		return nil, err
	}
	var result []model.ContainerResourceSample
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}
