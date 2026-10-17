package internal

import (
	"errors"
	"testing"
	"time"

	"github.com/gogu-x/ops/admin/internal/application"
	"github.com/gogu-x/ops/admin/internal/environment"
	"github.com/gogu-x/ops/admin/internal/project"
	"github.com/gogu-x/ops/admin/internal/service"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/ops/model"
)

type operationTestGateway struct{}

func (operationTestGateway) Request(_ string, message interface{}) (interface{}, error) {
	request, ok := message.(model.InstanceContainerActionRequest)
	if !ok {
		return nil, errors.New("unexpected request")
	}
	if request.ID == "instance-2" {
		return nil, errors.New("docker host unavailable")
	}
	return nil, nil
}

func TestBatchOperationTracksPartialResultsAndOwner(t *testing.T) {
	app := application.New(
		operationTestGateway{},
		project.NewMemoryRepository(),
		environment.NewMemoryRepository(),
		service.NewMemoryRepository(),
		instance.NewMemoryRepository(),
	)
	manager := newServiceOperationManager(app)
	initial, err := manager.start("user-1", "operator", model.ServiceInstanceBatchOperationRequest{
		InstanceIDs: []string{"instance-1", "instance-2"},
		Action:      "restart",
	}, []model.ServiceInstance{
		{ID: "instance-1", Name: "gate-1"},
		{ID: "instance-2", Name: "gate-2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if initial.Status != "running" || initial.Total != 2 {
		t.Fatalf("initial task state = %+v", initial)
	}

	deadline := time.Now().Add(2 * time.Second)
	var result model.ServiceInstanceOperation
	for time.Now().Before(deadline) {
		var ok bool
		result, ok = manager.get(initial.ID, "user-1")
		if ok && result.Status != "running" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if result.Status != "partial" || result.Completed != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("final task state = %+v", result)
	}
	if result.Items[1].Error != "docker host unavailable" {
		t.Fatalf("failure detail = %q", result.Items[1].Error)
	}
	if _, ok := manager.get(initial.ID, "another-user"); ok {
		t.Fatal("operation progress must only be visible to its owner")
	}
}

func TestBatchOperationRejectsBusyInstances(t *testing.T) {
	manager := newServiceOperationManager(nil)
	manager.operations["existing"] = &trackedServiceOperation{ServiceInstanceOperation: model.ServiceInstanceOperation{
		Status: "running",
		Items:  []model.ServiceInstanceOperationItem{{InstanceID: "instance-1", Status: "running"}},
	}}
	manager.busyIDs["instance-1"] = "existing"
	_, err := manager.start("user-1", "operator", model.ServiceInstanceBatchOperationRequest{
		InstanceIDs: []string{"instance-1"},
		Action:      "restart",
	}, []model.ServiceInstance{{ID: "instance-1", Name: "gate-1"}})
	if !errors.Is(err, errInstanceOperationBusy) {
		t.Fatalf("start error = %v", err)
	}
}
