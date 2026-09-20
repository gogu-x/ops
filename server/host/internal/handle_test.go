package internal

import (
	"reflect"
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestHostHandlersRegistered(t *testing.T) {
	service := NewDockerService()
	requests := []interface{}{
		model.ListRequest{},
		model.CreateRequest{},
		model.DeleteRequest{},
		model.TestRequest{},
		model.ContainerListRequest{},
		model.ContainerInspectRequest{},
		model.ContainerDeployRequest{},
		model.ContainerActionRequest{},
		model.ContainerLogsRequest{},
		model.ImagePullRequest{},
	}

	if len(service.handlers) != len(requests) {
		t.Fatalf("expected %d registered handlers, got %d", len(requests), len(service.handlers))
	}
	for _, request := range requests {
		if _, exists := service.handlers[reflect.TypeOf(request)]; !exists {
			t.Errorf("handler for %T is not registered", request)
		}
	}
}
