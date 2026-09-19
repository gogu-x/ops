package persistence

import (
	"context"
	"fmt"

	"github.com/gogu-x/ops/conf"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Mongo owns the process-wide MongoDB connection pool. Repositories receive
// its Database and never connect or disconnect independently.
type Mongo struct {
	client   *mongo.Client
	database *mongo.Database
}

func OpenMongo(ctx context.Context, cfg conf.Config) (*Mongo, error) {
	clientOptions := options.Client().ApplyURI(cfg.MongoURI)
	if cfg.MongoUsername != "" {
		clientOptions.SetAuth(options.Credential{Username: cfg.MongoUsername, Password: cfg.MongoPassword})
	}
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}
	return &Mongo{client: client, database: client.Database(cfg.MongoDatabase)}, nil
}

func (m *Mongo) Database() *mongo.Database { return m.database }

func (m *Mongo) Close(ctx context.Context) error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Disconnect(ctx)
}
