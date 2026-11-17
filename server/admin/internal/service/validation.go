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
	probe := item.ReadinessProbe
	probe.Type = strings.TrimSpace(probe.Type)
	switch probe.Type {
	case "", "docker":
	case "tcp", "http":
		if probe.Port < 1 || probe.Port > 65535 {
			return errors.New("readiness_probe.port must be between 1 and 65535")
		}
		if probe.Type == "http" && probe.Path != "" && !strings.HasPrefix(probe.Path, "/") {
			return errors.New("readiness_probe.path must start with /")
		}
	default:
		return errors.New("readiness_probe.type must be docker, tcp, http, or empty")
	}
	return nil
}
