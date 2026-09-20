package internal

import (
	"fmt"
	"reflect"

	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
)

type messageHandler func(tree.Context, interface{})

type handlerRegistry map[reflect.Type]messageHandler

func (is *InstanceService) registerHandlers() {
	is.handlers = make(handlerRegistry)

	registerHandler(is.handlers, is.handleEnrich)
	registerHandler(is.handlers, is.handleDeploy)
	registerHandler(is.handlers, is.handleUpdateImage)
	registerHandler(is.handlers, is.handleContainerAction)
	registerHandler(is.handlers, is.handleRemoveContainer)
	registerHandler(is.handlers, is.handleStatus)
	registerHandler(is.handlers, is.handleDetail)
	registerHandler(is.handlers, is.handleLogs)
	registerHandler(is.handlers, is.handleListEvents)
}

func registerHandler[T any](registry handlerRegistry, handler func(tree.Context, T)) {
	requestType := reflect.TypeOf((*T)(nil)).Elem()
	if _, exists := registry[requestType]; exists {
		panic(fmt.Sprintf("duplicate service instance handler for %s", requestType))
	}

	registry[requestType] = func(ctx tree.Context, message interface{}) {
		handler(ctx, message.(T))
	}
}

func (is *InstanceService) handleEnrich(ctx tree.Context, request model.EnrichRequest) {
	ctx.Response(model.ListResponse{ServiceInstances: is.enrichList(request.ServiceInstances)}, nil)
}

func (is *InstanceService) handleDeploy(ctx tree.Context, request model.DeployRequest) {
	result, err := is.deploy(request.ID)
	ctx.Response(result, err)
}

func (is *InstanceService) handleUpdateImage(ctx tree.Context, request model.UpdateImageRequest) {
	result, err := is.updateImage(request.ID, request.Image)
	ctx.Response(result, err)
}

func (is *InstanceService) handleContainerAction(ctx tree.Context, request model.InstanceContainerActionRequest) {
	err := is.containerAction(request.ID, request.Action)
	ctx.Response(nil, err)
}

func (is *InstanceService) handleRemoveContainer(ctx tree.Context, request model.RemoveContainerRequest) {
	err := is.removeContainer(request.ID)
	ctx.Response(nil, err)
}

func (is *InstanceService) handleStatus(ctx tree.Context, request model.StatusRequest) {
	result, err := is.status(request.ID)
	ctx.Response(result, err)
}

func (is *InstanceService) handleDetail(ctx tree.Context, request model.DetailRequest) {
	result, err := is.detail(request.ID)
	ctx.Response(model.DetailResponse{Detail: result}, err)
}

func (is *InstanceService) handleLogs(ctx tree.Context, request model.LogsRequest) {
	logs, err := is.logs(request.ID, request.Tail)
	ctx.Response(model.LogsResponse{Logs: logs}, err)
}

func (is *InstanceService) handleListEvents(ctx tree.Context, request model.ListEventsRequest) {
	events, err := is.listEvents(request.ID, request.Limit)
	ctx.Response(model.EventsResponse{Events: events}, err)
}
