package service

import (
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestServiceTypeValidation(t *testing.T) {
	if err := Validate(model.ServiceType{Name: "game"}); err == nil {
		t.Fatal("expected project_id validation error")
	}
	if err := Validate(model.ServiceType{ProjectID: "project-1", Name: "game"}); err == nil {
		t.Fatal("expected environment_id validation error")
	}
	if err := Validate(model.ServiceType{ProjectID: "project-1", EnvironmentID: "env-1", Name: "Game"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
}
