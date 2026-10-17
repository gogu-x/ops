package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogu-x/ops/model"
)

func TestDeploymentHistoryStoresResultAndOrder(t *testing.T) {
	repository := NewMemoryDeploymentRepository()
	actor := NewInstanceService()
	actor.deployments = repository
	actor.recordDeployment("instance-1", "update_image", "game:v2", "game:v1", "operator", nil)
	time.Sleep(time.Millisecond)
	actor.recordDeployment("instance-1", "update_image", "game:v3", "game:v2", "operator", errors.New("pull failed"))

	items, err := repository.ListByInstance(context.Background(), "instance-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Image != "game:v3" || items[0].PreviousImage != "game:v2" || items[0].Status != "failed" || items[0].Operator != "operator" {
		t.Fatalf("latest deployment = %+v", items)
	}
	all, err := repository.ListByInstance(context.Background(), "instance-1", 0)
	if err != nil || len(all) != 2 || all[1].Image != "game:v2" {
		t.Fatalf("deployment history = %+v, %v", all, err)
	}
}

func TestRollbackRejectsUnknownDeployment(t *testing.T) {
	actor := NewInstanceService()
	item := seedInstance(t, actor)
	_, err := actor.rollbackDeployment(item.ID, "unknown-deployment", "operator")
	if !errors.Is(err, model.ErrDeploymentUnavailable) {
		t.Fatalf("rollback error = %v", err)
	}
}

func TestDeploymentBaselineIsRecordedOnlyOnce(t *testing.T) {
	repository := NewMemoryDeploymentRepository()
	actor := NewInstanceService()
	actor.deployments = repository
	actor.recordDeploymentBaseline("instance-1", "game:v1")
	actor.recordDeploymentBaseline("instance-1", "game:v1")

	items, err := repository.ListByInstance(context.Background(), "instance-1", 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("baseline records = %+v, %v", items, err)
	}
	if items[0].Image != "game:v1" || items[0].Status != "succeeded" || items[0].Message == "" {
		t.Fatalf("baseline record = %+v", items[0])
	}
}
