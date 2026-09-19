package instance

import (
	"github.com/gogu-x/ops/instance/internal"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Repository is the service-instance persistence interface, re-exported at
// the package boundary so other top-level packages (e.g. admin/internal,
// which cannot import instance/internal directly) can depend on it.
type Repository = internal.Repository

// NewMongoRepository returns a Mongo-backed service-instance repository.
func NewMongoRepository(db *mongo.Database) Repository {
	return internal.NewMongoRepository(db)
}

// NewMemoryRepository returns an in-memory service-instance repository,
// useful for tests or when no database is configured.
func NewMemoryRepository() Repository {
	return internal.NewMemoryRepository()
}

// NewInstanceService constructs the service-instance actor. When
// mongodbData is non-nil, it persists instances to MongoDB; otherwise it
// falls back to an in-memory repository. The Docker host backing a service
// instance is resolved at request time via the ops-admin actor (see
// instance/internal/docker.go's resolveHostID), not injected here.
func NewInstanceService(mongodbData *mongo.Database) *internal.InstanceService {
	if mongodbData == nil {
		return internal.NewInstanceService()
	}
	return internal.NewInstanceService(internal.NewMongoRepository(mongodbData))
}
