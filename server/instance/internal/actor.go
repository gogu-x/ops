package internal

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
	treetimer "github.com/gogu-x/tree/timer"
)

type InstanceService struct {
	repo            Repository
	events          EventRepository
	deployments     DeploymentRepository
	resourceMetrics ResourceMetricsRepository
	handlers        handlerRegistry
	collectorCancel context.CancelFunc
	collectorCtx    context.Context
	resourceSystem  *tree.Tree
	resourceSelf    tree.PID
	resourceTimer   *treetimer.TimeWheel
}

func NewInstanceService(repositories ...Repository) *InstanceService {
	repo := Repository(NewMemoryRepository())
	if len(repositories) > 0 && repositories[0] != nil {
		repo = repositories[0]
	}
	service := &InstanceService{
		repo:            repo,
		events:          NewMemoryEventRepository(),
		deployments:     NewMemoryDeploymentRepository(),
		resourceMetrics: NewMemoryResourceMetricsRepository(),
	}
	service.registerHandlers()
	return service
}

// NewInstanceServiceWithEvents constructs the actor with an explicit event
// repository (e.g. Mongo-backed), falling back to an in-memory event
// repository when events is nil.
func NewInstanceServiceWithEvents(repo Repository, events EventRepository) *InstanceService {
	return NewInstanceServiceWithHistory(repo, events, nil)
}

// NewInstanceServiceWithHistory wires persistent lifecycle events and deployment history.
func NewInstanceServiceWithHistory(repo Repository, events EventRepository, deployments DeploymentRepository) *InstanceService {
	return NewInstanceServiceWithMonitoring(repo, events, deployments, NewMemoryResourceMetricsRepository())
}

// NewInstanceServiceWithMonitoring wires persistent instance history and
// resource samples. A nil resource repository falls back to memory storage.
func NewInstanceServiceWithMonitoring(repo Repository, events EventRepository, deployments DeploymentRepository, resources ResourceMetricsRepository) *InstanceService {
	a := NewInstanceService(repo)
	if events != nil {
		a.events = events
	}
	if deployments != nil {
		a.deployments = deployments
	}
	if resources != nil {
		a.resourceMetrics = resources
	}
	return a
}

// recordEvent best-effort records a lifecycle event for an instance. It
// never returns an error: event recording is diagnostic and must not affect
// the outcome of the operation it's attached to.
func (is *InstanceService) recordEvent(instanceID, eventType, message string) {
	if is.events == nil {
		return
	}
	_ = is.events.Create(bgCtx(), NewEvent(instanceID, eventType, message))
}

func (is *InstanceService) Name() string { return "ops-instance" }

func (is *InstanceService) OnInit(actorCtx tree.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	is.collectorCancel = cancel
	is.collectorCtx = ctx
	if indexes, ok := is.resourceMetrics.(interface{ EnsureIndexes(context.Context) error }); ok {
		indexCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		if err := indexes.EnsureIndexes(indexCtx); err != nil {
			log.Printf("[ops] initialize resource history indexes: %v", err)
		}
		stop()
	}
	is.startResourceCollector(actorCtx)
}

func (is *InstanceService) HandleMessage(ctx tree.Context, message interface{}) {
	handler, exists := is.handlers[reflect.TypeOf(message)]
	if !exists {
		ctx.Response(nil, fmt.Errorf("unsupported service instance message %T", message))
		return
	}

	handler(ctx, message)
}

func (is *InstanceService) OnStop(_ tree.Context) {
	if is.resourceTimer != nil {
		is.resourceTimer.Stop()
	}
	if is.collectorCancel != nil {
		is.collectorCancel()
	}
}

func bgCtx() context.Context { return context.Background() }

// Validate checks that a service instance's fields are well-formed. It
// delegates to model.ValidateServiceInstance, kept as a package-level
// function here for backward compatibility with existing call sites/tests.
func Validate(item model.ServiceInstance) error {
	return model.ValidateServiceInstance(item)
}
