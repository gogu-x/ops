package servicetype

import (
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestServiceTypeValidation(t *testing.T) {
	if err := Validate(model.ServiceType{Name: "game"}); err != nil {
		t.Fatalf("expected valid global type, got %v", err)
	}
	if err := Validate(model.ServiceType{Name: "Game"}); err == nil {
		t.Fatal("expected lowercase name validation error")
	}
}
