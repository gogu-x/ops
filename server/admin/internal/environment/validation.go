package environment

import (
	"errors"
	"strings"

	"github.com/gogu-x/ops/model"
)

func Validate(item model.Environment) error {
	if strings.TrimSpace(item.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}
