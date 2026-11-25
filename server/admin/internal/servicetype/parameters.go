package servicetype

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gogu-x/ops/model"
)

var ErrInvalidParameterValues = errors.New("启动参数无效")

// MergeParameterValues applies fields explicitly supplied by the form and validates
// the resulting values for the current service type.
func MergeParameterValues(fields []model.ServiceParameterField, current []model.ServiceParam, values map[string]string) ([]model.ServiceParam, error) {
	configured := make(map[string]model.ServiceParameterField, len(fields))
	for _, field := range fields {
		configured[field.Flag] = field
	}
	for flag := range values {
		field, ok := configured[flag]
		if field.Kind == "fixed" {
			return nil, fmt.Errorf("%w: %s 由服务类型统一指定", ErrInvalidParameterValues, flag)
		}
		if !ok {
			return nil, fmt.Errorf("%w: %s 未在服务类型中配置", ErrInvalidParameterValues, flag)
		}
	}
	params := make([]model.ServiceParam, 0, len(current)+len(values))
	existing := make(map[string]string, len(current))
	for _, param := range current {
		flag := strings.TrimSpace(param.Flag)
		if field := configured[flag]; field.Kind == "fixed" {
			if _, seen := existing[flag]; seen {
				continue
			}
			param.Value = field.DefaultValue
		}
		existing[flag] = param.Value
		if value, changed := values[flag]; !changed || (value == param.Value && value != "") {
			params = append(params, param)
		}
	}
	for _, field := range fields {
		if field.Kind == "fixed" {
			if _, ok := existing[field.Flag]; !ok {
				params = append(params, model.ServiceParam{Flag: field.Flag, Value: field.DefaultValue})
			}
			continue
		}
		value, supplied := values[field.Flag]
		if !supplied {
			value = existing[field.Flag]
		}
		if strings.TrimSpace(value) == "" {
			if field.Required {
				return nil, fmt.Errorf("%w: 请填写%s", ErrInvalidParameterValues, field.Label)
			}
			continue
		}
		switch field.Kind {
		case "integer":
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return nil, fmt.Errorf("%w: %s必须是整数", ErrInvalidParameterValues, field.Label)
			}
		case "select":
			if !slices.Contains(field.Options, value) {
				return nil, fmt.Errorf("%w: 请选择有效的%s", ErrInvalidParameterValues, field.Label)
			}
		}
		if supplied && value != existing[field.Flag] {
			params = append(params, model.ServiceParam{Flag: field.Flag, Value: value})
		}
	}
	return params, nil
}

// ParameterValuesMatch checks whether all explicit overrides are already stored.
func ParameterValuesMatch(params []model.ServiceParam, values map[string]string) bool {
	if len(values) == 0 {
		return true
	}
	current := make(map[string]string, len(params))
	for _, param := range params {
		current[strings.TrimSpace(param.Flag)] = param.Value
	}
	for flag, value := range values {
		if value != current[flag] {
			return false
		}
	}
	return true
}

// ValidateFixedArgs prevents free-form command arguments from overriding a type preset.
func ValidateFixedArgs(fields []model.ServiceParameterField, envText string) error {
	fixed := make(map[string]bool)
	for _, field := range fields {
		if field.Kind == "fixed" {
			fixed[strings.TrimLeft(field.Flag, "-")] = true
		}
	}
	if len(fixed) == 0 {
		return nil
	}
	for _, line := range strings.Split(envText, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "=") && !strings.HasPrefix(line, "--") {
			continue
		}
		for _, arg := range strings.Fields(line) {
			flag, _, _ := strings.Cut(arg, "=")
			if strings.HasPrefix(flag, "-") && fixed[strings.TrimLeft(flag, "-")] {
				return fmt.Errorf("%w: 请从附加参数中移除服务类型已固定的 %s", ErrInvalidParameterValues, flag)
			}
		}
	}
	return nil
}
