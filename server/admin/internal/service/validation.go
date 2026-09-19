package service

import (
	"errors"
	"regexp"
	"strings"

	"github.com/gogu-x/ops/model"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func Validate(item model.ServiceType) error {
	if strings.TrimSpace(item.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if strings.TrimSpace(item.EnvironmentID) == "" {
		return errors.New("environment_id is required")
	}
	if !serviceNamePattern.MatchString(item.Name) {
		return errors.New("name must contain only lowercase letters, numbers, _ or -")
	}
	return nil
}
