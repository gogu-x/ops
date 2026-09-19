package host

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

var ErrNotFound = errors.New("host not found")
var ErrAlreadyExists = errors.New("host already exists")

type Repository interface {
	List(ctx context.Context) ([]model.Host, error)
	Get(ctx context.Context, id string) (model.Host, error)
	Create(ctx context.Context, host model.Host) error
	Delete(ctx context.Context, id string) error
}

type MemoryRepository struct {
	mu    sync.RWMutex
	hosts map[string]model.Host
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{hosts: make(map[string]model.Host)}
}

func (r *MemoryRepository) List(_ context.Context) ([]model.Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Host, 0, len(r.hosts))
	for _, host := range r.hosts {
		result = append(result, host)
	}
	return result, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (model.Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	host, ok := r.hosts[id]
	if !ok {
		return model.Host{}, ErrNotFound
	}
	return host, nil
}

func (r *MemoryRepository) Create(_ context.Context, host model.Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.hosts {
		if existing.Name == host.Name {
			return ErrAlreadyExists
		}
	}
	r.hosts[host.ID] = host
	return nil
}

func (r *MemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.hosts[id]; !ok {
		return ErrNotFound
	}
	delete(r.hosts, id)
	return nil
}

type MongoRepository struct{ actor string }

func NewMongoRepository(actor string) *MongoRepository { return &MongoRepository{actor: actor} }

func (r *MongoRepository) List(context.Context) ([]model.Host, error) {
	var result []model.Host
	_, err := mongorpc.Request(r.actor, &mongorpc.FindMany{Collection: "hosts", Filter: bson.M{}, Results: &result})
	return result, err
}

func (r *MongoRepository) Get(_ context.Context, id string) (model.Host, error) {
	var host model.Host
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOne{Collection: "hosts", Filter: bson.M{"_id": id}, Result: &host})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.Host{}, ErrNotFound
	}
	return host, err
}

func (r *MongoRepository) Create(_ context.Context, host model.Host) error {
	_, err := mongorpc.Request(r.actor, &mongorpc.InsertOne{Collection: "hosts", Doc: host})
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Delete(_ context.Context, id string) error {
	v, err := mongorpc.Request(r.actor, &mongorpc.DeleteOne{Collection: "hosts", Filter: bson.M{"_id": id}})
	if err != nil {
		return err
	}
	if v.(mongorpc.WriteResult).DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func NewHost(name, dockerHost, tlsCA, tlsCert, tlsKey, note string) model.Host {
	now := time.Now().UTC()
	return model.Host{ID: uuid.NewString(), Name: name, DockerHost: dockerHost, TLSCA: tlsCA, TLSCert: tlsCert, TLSKey: tlsKey, Note: note, CreatedAt: now, UpdatedAt: now}
}
