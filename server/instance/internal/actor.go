package internal

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
)

type InstanceService struct {
	repo     Repository
	events   EventRepository
	handlers handlerRegistry
}

func NewInstanceService(repositories ...Repository) *InstanceService {
	repo := Repository(NewMemoryRepository())
	if len(repositories) > 0 && repositories[0] != nil {
		repo = repositories[0]
	}
	service := &InstanceService{repo: repo, events: NewMemoryEventRepository()}
	service.registerHandlers()
	return service
}

// NewInstanceServiceWithEvents constructs the actor with an explicit event
// repository (e.g. Mongo-backed), falling back to an in-memory event
// repository when events is nil.
func NewInstanceServiceWithEvents(repo Repository, events EventRepository) *InstanceService {
	a := NewInstanceService(repo)
	if events != nil {
		a.events = events
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

func (is *InstanceService) OnInit(_ tree.Context) {}

func (is *InstanceService) HandleMessage(ctx tree.Context, message interface{}) {
	handler, exists := is.handlers[reflect.TypeOf(message)]
	if !exists {
		ctx.Response(nil, fmt.Errorf("unsupported service instance message %T", message))
		return
	}

	handler(ctx, message)
}

func (is *InstanceService) OnStop(_ tree.Context) {}

func bgCtx() context.Context { return context.Background() }

// Validate checks that a service instance's fields are well-formed. It
// delegates to model.ValidateServiceInstance, kept as a package-level
// function here for backward compatibility with existing call sites/tests.
func Validate(item model.ServiceInstance) error {
	return model.ValidateServiceInstance(item)
}
