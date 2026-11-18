package servicetype

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gogu-x/ops/model"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func Validate(item model.ServiceType) error {
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
	return validateParameterFields(item.ParameterFields)
}

func validateParameterFields(fields []model.ServiceParameterField) error {
	seen := make(map[string]struct{}, len(fields))
	for i, field := range fields {
		if strings.TrimSpace(field.Label) == "" || !model.ValidServiceParamFlag(field.Flag) {
			return fmt.Errorf("第 %d 个字段需要显示名称和有效的启动参数，例如 --max-players", i+1)
		}
		key := strings.TrimLeft(field.Flag, "-")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("启动参数 %s 重复", field.Flag)
		}
		seen[key] = struct{}{}
		switch field.Kind {
		case "text":
		case "integer":
			if field.DefaultValue != "" {
				if _, err := strconv.ParseInt(field.DefaultValue, 10, 64); err != nil {
					return fmt.Errorf("%s 的默认值必须是整数", field.Flag)
				}
			}
		case "select":
			options := make(map[string]struct{}, len(field.Options))
			for _, option := range field.Options {
				_, duplicate := options[option]
				if strings.TrimSpace(option) == "" || duplicate {
					return fmt.Errorf("%s 的选项不能为空或重复", field.Flag)
				}
				options[option] = struct{}{}
			}
			if len(options) == 0 {
				return fmt.Errorf("%s 至少需要一个选项", field.Flag)
			}
			if _, exists := options[field.DefaultValue]; field.DefaultValue != "" && !exists {
				return fmt.Errorf("%s 的默认值必须是可选项之一", field.Flag)
			}
		default:
			return fmt.Errorf("%s 的输入类型无效", field.Flag)
		}
	}
	return nil
}
