package ops

import (
	"context"
	"log"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/instance"
	"github.com/gogu-x/ops/ops/internal"
	"github.com/gogu-x/ops/ops/internal/application"
	"github.com/gogu-x/ops/ops/internal/persistence"
	"github.com/gogu-x/ops/ops/internal/project"
	"github.com/gogu-x/ops/ops/internal/service"
	"github.com/gogu-x/ops/ops/internal/store"
	"github.com/gogu-x/ops/ops/mongorpc"
	"github.com/gogu-x/tree"
)

// System owns process-wide infrastructure and the actors that perform HTTP
// serving and long-running Docker work.
type System struct {
	Actors []tree.Actor
	mongo  *persistence.Mongo
}

func NewSystem(ctx context.Context, cfg conf.Config) (*System, error) {
	var (
		authRepo     store.Repository    = store.NewMemoryRepository()
		hostRepo     host.Repository     = host.NewMemoryRepository()
		projectRepo  project.Repository  = project.NewMemoryRepository()
		serviceRepo  service.Repository  = service.NewMemoryRepository()
		instanceRepo instance.Repository = instance.NewMemoryRepository()
		mongoStore   *persistence.Mongo
	)

	if cfg.MongoURI != "" {
		var err error
		mongoStore, err = persistence.OpenMongo(ctx, cfg)
		if err != nil {
			return nil, err
		}
		authRepo = store.NewMongoRepository(mongorpc.DefaultActorName)
		hostRepo = host.NewMongoRepository(mongorpc.DefaultActorName)
		projectRepo = project.NewMongoRepository(mongorpc.DefaultActorName)
		serviceRepo = service.NewMongoRepository(mongorpc.DefaultActorName)
		instanceRepo = instance.NewMongoRepository(mongorpc.DefaultActorName)
	} else {
		log.Println("[ops] OPS_MONGO_URI 未设置，使用内存仓储；生产环境请配置 MongoDB")
	}

	gateway := application.NewTreeGateway(20 * time.Second)
	app := application.New(gateway, projectRepo, serviceRepo, instanceRepo)
	actors := make([]tree.Actor, 0, 4)
	if mongoStore != nil {
		mongoActor := mongorpc.NewActor(mongorpc.DefaultActorName, mongoStore.Database())
		actors = append(actors, mongoActor)
	}
	actors = append(actors,
		internal.NewActor(cfg, authRepo, app),
		host.NewActor(cfg, hostRepo),
		instance.NewActorWithRepositories(cfg, instanceRepo, serviceRepo),
	)
	return &System{
		Actors: actors,
		mongo:  mongoStore,
	}, nil
}

func (s *System) Close(ctx context.Context) error {
	if s == nil || s.mongo == nil {
		return nil
	}
	return s.mongo.Close(ctx)
}
