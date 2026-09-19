package host

import (
	"github.com/gogu-x/ops/host/internal"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func NewHostService(mongodbData *mongo.Database) *internal.DockerService {
	if mongodbData == nil {
		return internal.NewDockerService()
	}
	return internal.NewDockerService(internal.NewMongoRepository(mongodbData))
}
