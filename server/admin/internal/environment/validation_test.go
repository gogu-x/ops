package environment

import (
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestEnvironmentValidation(t *testing.T) {
	if err := Validate(model.Environment{Name: "beta"}); err == nil {
		t.Fatal("expected project_id required validation error")
	}
	if err := Validate(model.Environment{ProjectID: "project-1", Name: ""}); err == nil {
		t.Fatal("expected name required validation error")
	}
}
