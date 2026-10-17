package internal

import (
	"reflect"
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestInstanceHandlersRegistered(t *testing.T) {
	service := NewInstanceService()
	requests := []interface{}{
		model.EnrichRequest{},
		model.DeployRequest{},
		model.UpdateImageRequest{},
		model.InstanceContainerActionRequest{},
		model.RemoveContainerRequest{},
		model.StatusRequest{},
		model.DetailRequest{},
		model.LogsRequest{},
		model.InstanceResourceHistoryRequest{},
		model.ListEventsRequest{},
		model.ListDeploymentsRequest{},
		model.RollbackDeploymentRequest{},
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
