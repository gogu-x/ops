package store

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
}

// MemoryRepository is an in-memory implementation of Repository, useful for
// tests or when no database is configured.
type MemoryRepository struct {
	mu            sync.RWMutex
	usersByID     map[string]model.User
	refreshTokens map[string]model.RefreshToken
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		usersByID:     make(map[string]model.User),
		refreshTokens: make(map[string]model.RefreshToken),
	}
}

func (r *MemoryRepository) FindUser(_ context.Context, username string) (model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, user := range r.usersByID {
		if user.Username == username {
			return user, nil
		}
	}
	return model.User{}, ErrNotFound
}

func (r *MemoryRepository) FindUserByID(_ context.Context, id string) (model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.usersByID[id]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return user, nil
}

func (r *MemoryRepository) CreateUser(_ context.Context, user model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.usersByID {
		if existing.Username == user.Username {
			return ErrAlreadyExists
		}
	}
	r.usersByID[user.ID] = user
	return nil
}

func (r *MemoryRepository) CountUsers(context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.usersByID)), nil
}

func (r *MemoryRepository) SaveRefreshToken(_ context.Context, token model.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshTokens[token.ID] = token
	return nil
}

func (r *MemoryRepository) ConsumeRefreshToken(_ context.Context, tokenHash string, now time.Time) (model.RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, token := range r.refreshTokens {
		if token.TokenHash == tokenHash && !token.Revoked && token.ExpiresAt.After(now) {
			token.Revoked = true
			r.refreshTokens[id] = token
			return token, nil
		}
	}
	return model.RefreshToken{}, ErrNotFound
}

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) *MongoRepository { return &MongoRepository{db: db} }

func (r *MongoRepository) users() *mongo.Collection         { return r.db.Collection("users") }
func (r *MongoRepository) refreshTokens() *mongo.Collection { return r.db.Collection("refresh_tokens") }

func (r *MongoRepository) FindUser(ctx context.Context, username string) (model.User, error) {
	var user model.User
	err := r.users().FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return user, nil
}

func (r *MongoRepository) FindUserByID(ctx context.Context, id string) (model.User, error) {
	var user model.User
	err := r.users().FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	return user, nil
}

func (r *MongoRepository) CreateUser(ctx context.Context, user model.User) error {
	_, err := r.users().InsertOne(ctx, user)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) CountUsers(ctx context.Context) (int64, error) {
	return r.users().CountDocuments(ctx, bson.M{})
}

func (r *MongoRepository) SaveRefreshToken(ctx context.Context, token model.RefreshToken) error {
	_, err := r.refreshTokens().InsertOne(ctx, token)
	return err
}

func (r *MongoRepository) ConsumeRefreshToken(ctx context.Context, tokenHash string, now time.Time) (model.RefreshToken, error) {
	var token model.RefreshToken
	filter := bson.M{"token_hash": tokenHash, "revoked": false, "expires_at": bson.M{"$gt": now}}
	update := bson.M{"$set": bson.M{"revoked": true}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.refreshTokens().FindOneAndUpdate(ctx, filter, update, opts).Decode(&token)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.RefreshToken{}, ErrNotFound
		}
		return model.RefreshToken{}, err
	}
	return token, nil
}

func NewUser(username, passwordHash, role string) model.User {
	return model.User{ID: uuid.NewString(), Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: time.Now().UTC()}
}
