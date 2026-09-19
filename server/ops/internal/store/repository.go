package store

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

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

type Repository interface {
	FindUser(ctx context.Context, username string) (model.User, error)
	FindUserByID(ctx context.Context, id string) (model.User, error)
	CreateUser(ctx context.Context, user model.User) error
	CountUsers(ctx context.Context) (int64, error)
	SaveRefreshToken(ctx context.Context, token model.RefreshToken) error
	ConsumeRefreshToken(ctx context.Context, tokenHash string, now time.Time) (model.RefreshToken, error)
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

type MongoRepository struct{ actor string }

func NewMongoRepository(actor string) *MongoRepository { return &MongoRepository{actor: actor} }

func (r *MongoRepository) FindUser(_ context.Context, username string) (model.User, error) {
	var user model.User
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOne{Collection: "users", Filter: bson.M{"username": username}, Result: &user})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.User{}, ErrNotFound
	}
	return user, err
}

func (r *MongoRepository) FindUserByID(_ context.Context, id string) (model.User, error) {
	var user model.User
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOne{Collection: "users", Filter: bson.M{"_id": id}, Result: &user})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.User{}, ErrNotFound
	}
	return user, err
}

func (r *MongoRepository) CreateUser(_ context.Context, user model.User) error {
	_, err := mongorpc.Request(r.actor, &mongorpc.InsertOne{Collection: "users", Doc: user})
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) CountUsers(context.Context) (int64, error) {
	v, err := mongorpc.Request(r.actor, &mongorpc.Count{Collection: "users", Filter: bson.M{}})
	if err != nil {
		return 0, err
	}
	return v.(int64), nil
}

func (r *MongoRepository) SaveRefreshToken(_ context.Context, token model.RefreshToken) error {
	_, err := mongorpc.Request(r.actor, &mongorpc.InsertOne{Collection: "refresh_tokens", Doc: token})
	return err
}

func (r *MongoRepository) ConsumeRefreshToken(_ context.Context, tokenHash string, now time.Time) (model.RefreshToken, error) {
	var token model.RefreshToken
	_, err := mongorpc.Request(r.actor, &mongorpc.FindOneAndUpdate{Collection: "refresh_tokens", Filter: bson.M{"token_hash": tokenHash, "revoked": false, "expires_at": bson.M{"$gt": now}}, Update: bson.M{"$set": bson.M{"revoked": true}}, Result: &token})
	if err != nil && strings.Contains(err.Error(), "no documents") {
		return model.RefreshToken{}, ErrNotFound
	}
	return token, err
}

func NewUser(username, passwordHash, role string) model.User {
	return model.User{ID: uuid.NewString(), Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: time.Now().UTC()}
}
