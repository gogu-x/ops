package servicetype

import (
	"context"
	"errors"
	"strings"

	"github.com/gogu-x/ops/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrNotFound = errors.New("service type not found")
var ErrAlreadyExists = errors.New("service type already exists")

type Repository interface {
	List(ctx context.Context) ([]model.ServiceType, error)
	Get(ctx context.Context, id string) (model.ServiceType, error)
	Create(ctx context.Context, item model.ServiceType) error
	Update(ctx context.Context, item model.ServiceType) error
	Delete(ctx context.Context, id string) error
}

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) *MongoRepository { return &MongoRepository{db: db} }

func (r *MongoRepository) collection() *mongo.Collection { return r.db.Collection("service_types") }

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	return err
}

func (r *MongoRepository) List(ctx context.Context) ([]model.ServiceType, error) {
	var result []model.ServiceType
	cursor, err := r.collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *MongoRepository) Get(ctx context.Context, id string) (model.ServiceType, error) {
	var item model.ServiceType
	err := r.collection().FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) || strings.Contains(err.Error(), "no documents") {
			return model.ServiceType{}, ErrNotFound
		}
		return model.ServiceType{}, err
	}
	return item, nil
}

func (r *MongoRepository) Create(ctx context.Context, item model.ServiceType) error {
	_, err := r.collection().InsertOne(ctx, item)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyExists
	}
	return err
}

func (r *MongoRepository) Update(ctx context.Context, item model.ServiceType) error {
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
