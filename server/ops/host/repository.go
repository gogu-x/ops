package host

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

var ErrNotFound = errors.New("host not found")
var ErrAlreadyExists = errors.New("host already exists")

type Repository interface {
	List(ctx context.Context) ([]model.Host, error)
	Get(ctx context.Context, id string) (model.Host, error)
	Create(ctx context.Context, host model.Host) error
	Delete(ctx context.Context, id string) error
	Close(ctx context.Context) error
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

func (r *MemoryRepository) Close(context.Context) error { return nil }

type MongoRepository struct {
	client *mongo.Client
	hosts  *mongo.Collection
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
	return &MongoRepository{client: client, hosts: client.Database(cfg.MongoDatabase).Collection("hosts")}, nil
}

func (r *MongoRepository) List(ctx context.Context) ([]model.Host, error) {
	cursor, err := r.hosts.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var result []model.Host
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.Host, error) {
	var host model.Host
	if err := r.hosts.FindOne(ctx, bson.M{"_id": id}).Decode(&host); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.Host{}, ErrNotFound
		}
		return model.Host{}, err
	}
	return host, nil
}

func (r *MongoRepository) Create(ctx context.Context, host model.Host) error {
	_, err := r.hosts.InsertOne(ctx, host)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.hosts.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MongoRepository) Close(ctx context.Context) error { return r.client.Disconnect(ctx) }

func NewHost(name, dockerHost, tlsCA, tlsCert, tlsKey, note string) model.Host {
	now := time.Now().UTC()
	return model.Host{ID: uuid.NewString(), Name: name, DockerHost: dockerHost, TLSCA: tlsCA, TLSCert: tlsCert, TLSKey: tlsKey, Note: note, CreatedAt: now, UpdatedAt: now}
}
