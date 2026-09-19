package project

import (
	"testing"

	"github.com/gogu-x/ops/ops/internal/model"
)

func TestProjectValidation(t *testing.T) {
	if err := Validate(model.Project{Name: ""}); err == nil {
		t.Fatal("expected name required validation error")
	}
}
