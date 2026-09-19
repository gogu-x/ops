package service

import (
	"testing"

	"github.com/gogu-x/ops/ops/internal/model"
)

func TestServiceTypeValidation(t *testing.T) {
	if err := Validate(model.ServiceType{HostID: "host-1", Name: "game"}); err == nil {
		t.Fatal("expected project_id validation error")
	}
	if err := Validate(model.ServiceType{ProjectID: "project-1", HostID: "host-1", Name: "Game"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
}
