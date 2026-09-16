package store

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

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

type Repository interface {
	FindUser(ctx context.Context, username string) (model.User, error)
	FindUserByID(ctx context.Context, id string) (model.User, error)
	CreateUser(ctx context.Context, user model.User) error
	CountUsers(ctx context.Context) (int64, error)
	SaveRefreshToken(ctx context.Context, token model.RefreshToken) error
	ConsumeRefreshToken(ctx context.Context, tokenHash string, now time.Time) (model.RefreshToken, error)
	Close(ctx context.Context) error
}

type MemoryRepository struct {
	mu     sync.RWMutex
	users  map[string]model.User
	tokens map[string]model.RefreshToken
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{users: make(map[string]model.User), tokens: make(map[string]model.RefreshToken)}
}

func (r *MemoryRepository) FindUser(_ context.Context, username string) (model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, user := range r.users {
		if user.Username == username {
			return user, nil
		}
	}
	return model.User{}, ErrNotFound
}

func (r *MemoryRepository) FindUserByID(_ context.Context, id string) (model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return user, nil
}

func (r *MemoryRepository) CreateUser(_ context.Context, user model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.users {
		if existing.Username == user.Username {
			return ErrAlreadyExists
		}
	}
	r.users[user.ID] = user
	return nil
}

func (r *MemoryRepository) CountUsers(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.users)), nil
}

func (r *MemoryRepository) SaveRefreshToken(_ context.Context, token model.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *MemoryRepository) ConsumeRefreshToken(_ context.Context, tokenHash string, now time.Time) (model.RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenHash]
	if !ok || token.Revoked || token.ExpiresAt.Before(now) {
		return model.RefreshToken{}, ErrNotFound
	}
	token.Revoked = true
	r.tokens[tokenHash] = token
	return token, nil
}

func (r *MemoryRepository) Close(context.Context) error { return nil }

type MongoRepository struct {
	client *mongo.Client
	users  *mongo.Collection
	tokens *mongo.Collection
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
	db := client.Database(cfg.MongoDatabase)
	return &MongoRepository{client: client, users: db.Collection("users"), tokens: db.Collection("refresh_tokens")}, nil
}

func (r *MongoRepository) FindUser(ctx context.Context, username string) (model.User, error) {
	var user model.User
	err := r.users.FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, ErrNotFound
	}
	return user, err
}

func (r *MongoRepository) FindUserByID(ctx context.Context, id string) (model.User, error) {
	var user model.User
	err := r.users.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, ErrNotFound
	}
	return user, err
}

func (r *MongoRepository) CreateUser(ctx context.Context, user model.User) error {
	_, err := r.users.InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) CountUsers(ctx context.Context) (int64, error) {
	return r.users.CountDocuments(ctx, bson.M{})
}

func (r *MongoRepository) SaveRefreshToken(ctx context.Context, token model.RefreshToken) error {
	_, err := r.tokens.InsertOne(ctx, token)
	return err
}

func (r *MongoRepository) ConsumeRefreshToken(ctx context.Context, tokenHash string, now time.Time) (model.RefreshToken, error) {
	var token model.RefreshToken
	result := r.tokens.FindOneAndUpdate(ctx, bson.M{"token_hash": tokenHash, "revoked": false, "expires_at": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"revoked": true}})
	if err := result.Decode(&token); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.RefreshToken{}, ErrNotFound
		}
		return model.RefreshToken{}, err
	}
	return token, nil
}

func (r *MongoRepository) Close(ctx context.Context) error { return r.client.Disconnect(ctx) }

func NewUser(username, passwordHash, role string) model.User {
	return model.User{ID: uuid.NewString(), Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: time.Now().UTC()}
}
