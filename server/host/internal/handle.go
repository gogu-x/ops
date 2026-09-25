package internal

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
)

type messageHandler func(tree.Context, interface{})

type handlerRegistry map[reflect.Type]messageHandler

func (a *DockerService) registerHandlers() {
	a.handlers = make(handlerRegistry)

	registerHandler(a.handlers, a.handleList)
	registerHandler(a.handlers, a.handleCreate)
	registerHandler(a.handlers, a.handleDelete)
	registerHandler(a.handlers, a.handleSetProjectHosts)
	registerHandler(a.handlers, a.handleTest)
	registerHandler(a.handlers, a.handleContainerList)
	registerHandler(a.handlers, a.handleContainerInspect)
	registerHandler(a.handlers, a.handleContainerDeploy)
	registerHandler(a.handlers, a.handleContainerAction)
	registerHandler(a.handlers, a.handleContainerLogs)
	registerHandler(a.handlers, a.handleImagePull)
}

func registerHandler[T any](registry handlerRegistry, handler func(tree.Context, T)) {
	requestType := reflect.TypeOf((*T)(nil)).Elem()
	if _, exists := registry[requestType]; exists {
		panic(fmt.Sprintf("duplicate host handler for %s", requestType))
	}

	registry[requestType] = func(ctx tree.Context, message interface{}) {
		handler(ctx, message.(T))
	}
}

func (a *DockerService) HandleMessage(ctx tree.Context, message interface{}) {
	handler, exists := a.handlers[reflect.TypeOf(message)]
	if !exists {
		ctx.Response(nil, fmt.Errorf("unsupported host message %T", message))
		return
	}

	handler(ctx, message)
}

func (a *DockerService) handleList(ctx tree.Context, _ model.ListRequest) {
	hosts, err := a.repo.List(context.Background())
	for i := range hosts {
		hosts[i] = publicHost(hosts[i])
	}
	ctx.Response(model.HostListResponse{Hosts: hosts}, err)
}

func (a *DockerService) handleCreate(ctx tree.Context, request model.CreateRequest) {
	created, err := a.create(request.Host)
	ctx.Response(publicHost(created), err)
}

func (a *DockerService) handleDelete(ctx tree.Context, request model.DeleteRequest) {
	err := a.delete(request.ID)
	ctx.Response(nil, err)
}

func (a *DockerService) handleSetProjectHosts(ctx tree.Context, request model.SetProjectHostsRequest) {
	err := a.setProjectHosts(request.ProjectID, request.HostIDs)
	ctx.Response(nil, err)
}

func (a *DockerService) handleTest(ctx tree.Context, request model.TestRequest) {
	result, err := a.test(request.ID)
	ctx.Response(result, err)
}

func (a *DockerService) handleContainerList(ctx tree.Context, request model.ContainerListRequest) {
	result, err := a.containerList(request.HostID)
	ctx.Response(result, err)
}

func (a *DockerService) handleContainerInspect(ctx tree.Context, request model.ContainerInspectRequest) {
	result, err := a.containerInspect(request.HostID, request.ContainerID)
	ctx.Response(result, err)
}

func (a *DockerService) handleContainerDeploy(ctx tree.Context, request model.ContainerDeployRequest) {
	result, err := a.containerDeploy(request.HostID, request.Spec)
	ctx.Response(result, err)
}

func (a *DockerService) handleContainerAction(ctx tree.Context, request model.ContainerActionRequest) {
	err := a.containerAction(request.HostID, request.ContainerID, request.Action)
	ctx.Response(nil, err)
}

func (a *DockerService) handleContainerLogs(ctx tree.Context, request model.ContainerLogsRequest) {
	result, err := a.containerLogs(request.HostID, request.ContainerID, request.Tail)
	ctx.Response(result, err)
}

func (a *DockerService) handleImagePull(ctx tree.Context, request model.ImagePullRequest) {
	err := a.imagePull(request.HostID, request.Image)
	ctx.Response(nil, err)
}
