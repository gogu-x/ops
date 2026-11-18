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
	configured := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		configured[field.Flag] = struct{}{}
	}
	for flag := range values {
		if _, ok := configured[flag]; !ok {
			return nil, fmt.Errorf("%w: %s 未在服务类型中配置", ErrInvalidParameterValues, flag)
		}
	}
	params := make([]model.ServiceParam, 0, len(current)+len(values))
	existing := make(map[string]string, len(current))
	for _, param := range current {
		flag := strings.TrimSpace(param.Flag)
		existing[flag] = param.Value
		if _, changed := values[flag]; !changed {
			params = append(params, param)
		}
	}
	for _, field := range fields {
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
		if supplied {
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
