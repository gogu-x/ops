package internal

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

// ErrNotFound and ErrAlreadyExists alias the shared model sentinels so that
// admin/internal's HTTP handlers (which cannot import this internal
// package) can distinguish error cases via model.ErrNotFound /
// model.ErrAlreadyExists, while existing call sites within this package
// keep using the unqualified names.
var ErrNotFound = model.ErrNotFound
var ErrAlreadyExists = model.ErrAlreadyExists

type Repository interface {
	List(ctx context.Context) ([]model.Host, error)
	Get(ctx context.Context, id string) (model.Host, error)
	Create(ctx context.Context, host model.Host) error
	Delete(ctx context.Context, id string) error
	SetProjectHosts(ctx context.Context, projectID string, hostIDs []string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]model.Host
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]model.Host)}
}

func (r *MemoryRepository) List(context.Context) ([]model.Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Host, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return model.Host{}, ErrNotFound
	}
	return item, nil
}

func (r *MemoryRepository) Create(_ context.Context, host model.Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.Name == host.Name {
			return ErrAlreadyExists
		}
	}
	r.items[host.ID] = host
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

func (r *MemoryRepository) SetProjectHosts(_ context.Context, projectID string, hostIDs []string) error {
	wanted := make(map[string]struct{}, len(hostIDs))
	for _, id := range hostIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range wanted {
		if _, ok := r.items[id]; !ok {
			return ErrNotFound
		}
	}

	now := time.Now().UTC()
	for id, host := range r.items {
		if _, keep := wanted[id]; keep || !host.HasProject(projectID) {
			continue
		}
		host.ProjectIDs = removeProjectID(host.ProjectIDs, projectID)
		if strings.TrimSpace(host.ProjectID) == projectID {
			host.ProjectID = ""
		}
		host.UpdatedAt = now
		r.items[id] = host
	}
	for id := range wanted {
		host := r.items[id]
		if !host.HasProject(projectID) {
			host.ProjectIDs = append(host.ProjectIDs, projectID)
		}
		host.UpdatedAt = now
		r.items[id] = host
	}
	return nil
}

func removeProjectID(projectIDs []string, projectID string) []string {
	kept := projectIDs[:0]
	for _, id := range projectIDs {
		if strings.TrimSpace(id) != projectID {
			kept = append(kept, id)
		}
	}
	return kept
}

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) *MongoRepository { return &MongoRepository{db: db} }

func (r *MongoRepository) collection() *mongo.Collection { return r.db.Collection("hosts") }

func (r *MongoRepository) List(ctx context.Context) ([]model.Host, error) {
	var result []model.Host
	cursor, err := r.collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.Host, error) {
	var host model.Host
	err := r.collection().FindOne(ctx, bson.M{"_id": id}).Decode(&host)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.Host{}, ErrNotFound
		}
		return model.Host{}, err
	}
	return host, nil
}

func (r *MongoRepository) Create(ctx context.Context, host model.Host) error {
	_, err := r.collection().InsertOne(ctx, host)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
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

func (r *MongoRepository) SetProjectHosts(ctx context.Context, projectID string, hostIDs []string) error {
	uniqueIDs := make([]string, 0, len(hostIDs))
	seen := make(map[string]struct{}, len(hostIDs))
	for _, id := range hostIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}

	if len(uniqueIDs) > 0 {
		cursor, err := r.collection().Find(ctx, bson.M{"_id": bson.M{"$in": uniqueIDs}})
		if err != nil {
			return err
		}
		defer cursor.Close(ctx)
		var selected []model.Host
		if err := cursor.All(ctx, &selected); err != nil {
			return err
		}
		if len(selected) != len(uniqueIDs) {
			return ErrNotFound
		}
		result, err := r.collection().UpdateMany(ctx,
			bson.M{"_id": bson.M{"$in": uniqueIDs}},
			bson.M{
				"$addToSet": bson.M{"project_ids": projectID},
				"$set":      bson.M{"updated_at": time.Now().UTC()},
			},
		)
		if err != nil {
			return err
		}
		if result.MatchedCount != int64(len(uniqueIDs)) {
			return ErrNotFound
		}
	}

	filter := bson.M{"project_ids": projectID}
	if len(uniqueIDs) > 0 {
		filter["_id"] = bson.M{"$nin": uniqueIDs}
	}
	if _, err := r.collection().UpdateMany(ctx, filter, bson.M{
		"$pull": bson.M{"project_ids": projectID},
		"$set":  bson.M{"updated_at": time.Now().UTC()},
	}); err != nil {
		return err
	}
	legacyFilter := bson.M{"project_id": projectID}
	if len(uniqueIDs) > 0 {
		legacyFilter["_id"] = bson.M{"$nin": uniqueIDs}
	}
	_, err := r.collection().UpdateMany(ctx, legacyFilter, bson.M{"$set": bson.M{"project_id": "", "updated_at": time.Now().UTC()}})
	return err
}

func NewHost(name, dockerHost, tlsCA, tlsCert, tlsKey, note string) model.Host {
	now := time.Now().UTC()
	return model.Host{ID: uuid.NewString(), Name: name, DockerHost: dockerHost, TLSCA: tlsCA, TLSCert: tlsCert, TLSKey: tlsKey, Note: note, CreatedAt: now, UpdatedAt: now}
}
