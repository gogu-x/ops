package project

import (
	"errors"
	"strings"

	"github.com/gogu-x/ops/ops/internal/model"
)

func Validate(item model.Project) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}
