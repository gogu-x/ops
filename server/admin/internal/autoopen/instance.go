package autoopen

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gogu-x/ops/model"
	"github.com/google/uuid"
)

const defaultGameBasePort = 9900

var gameConnectionFlags = [...]string{
	"--node-id", "--etcd", "--nats-url",
	"--mongo-url", "--mongo-username", "--mongo-password",
	"--gconf-db-uri", "--gconf-db", "--platform-grpc-addr",
}

func templateParameters(item model.ServiceInstance) []model.AutoOpenTemplateParam {
	params := make([]model.AutoOpenTemplateParam, 0, len(gameConnectionFlags))
	for _, flag := range gameConnectionFlags {
		value := gameFlagValue(item, flag)
		param := model.AutoOpenTemplateParam{Flag: flag, Configured: value != ""}
		switch flag {
		case "--mongo-password":
		case "--etcd", "--nats-url", "--mongo-url", "--gconf-db-uri", "--platform-grpc-addr":
			param.DisplayValue = connectionAddress(value)
		default:
			param.DisplayValue = value
		}
		params = append(params, param)
	}
	return params
}

// Only display the network address. User info, paths, queries and ambiguous
// connection lists can contain credentials.
func connectionAddress(value string) string {
	if value == "" || strings.Contains(value, ",") {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "//" + value
	}
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Host == "" {
		return ""
	}
	if endpoint.Scheme == "" {
		return endpoint.Host
	}
	return endpoint.Scheme + "://" + endpoint.Host
}

func instanceServerID(item model.ServiceInstance) int {
	value := gameFlagValue(item, "--server-id")
	if value == "" {
		return 1
	}
	id, _ := strconv.Atoi(value)
	return id
}

func scheduledInstanceID(policy model.AutoOpenPolicy, serverID int) string {
	scope := fmt.Sprintf("ops-auto-open/%s/%s/%d", policy.ProjectID, policy.EnvironmentID, serverID)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(scope)).String()
}

func scheduledInstance(template model.ServiceInstance, policy model.AutoOpenPolicy, index int, openAt int64) (model.ServiceInstance, error) {
	serverID := policy.FirstServerID + index
	containerPort := defaultGameBasePort
	if value := gameFlagValue(template, "--game-base-port"); value != "" {
		base, err := strconv.Atoi(value)
		if err != nil || base < 0 || base > 65535 {
			return model.ServiceInstance{}, fmt.Errorf("模板的 --game-base-port 无效")
		}
		containerPort = base
	}
	containerPort += serverID
	if containerPort < 1 || containerPort > 65535 {
		return model.ServiceInstance{}, fmt.Errorf("区服 %d 的 gRPC 端口超出 1 到 65535", serverID)
	}

	item := template
	item.Name = policy.NamePrefix + strconv.Itoa(serverID)
	item.Network = strings.TrimSpace(item.Network)
	switch item.Network {
	case "", "bridge", "host", "none":
		return model.ServiceInstance{}, fmt.Errorf("请选择使用自定义 Docker 网络的 Game 实例作为模板")
	}
	item.Params = make([]model.ServiceParam, 0, len(template.Params)+3)
	for _, param := range template.Params {
		if !scheduledFlag(strings.TrimSpace(param.Flag)) {
			item.Params = append(item.Params, param)
		}
	}
	item.Params = append(item.Params,
		model.ServiceParam{Flag: "--server-id", Value: strconv.Itoa(serverID)},
		model.ServiceParam{Flag: "--server-open-ts", Value: strconv.FormatInt(openAt, 10)},
		model.ServiceParam{Flag: "--grpc-host", Value: item.Name},
	)
	item.EnvText = withoutScheduledFlags(template.EnvText)
	item.PortMapping = ""
	if policy.FirstHostPort > 0 {
		item.PortMapping = fmt.Sprintf("%d:%d", policy.FirstHostPort+index, containerPort)
	}
	return item, model.ValidateServiceInstance(item)
}

func scheduledFlag(flag string) bool {
	switch flag {
	case "--server-id", "--server-open-ts", "--grpc-host":
		return true
	default:
		return false
	}
}

func gameFlagValue(item model.ServiceInstance, target string) string {
	var value string
	for _, param := range item.Params {
		if strings.TrimSpace(param.Flag) == target {
			value = strings.TrimSpace(param.Value)
		}
	}
	for _, line := range strings.Split(item.EnvText, "\n") {
		if isEnvAssignment(line) {
			continue
		}
		fields := launchFields(line)
		for i, token := range fields {
			flag, inline, hasValue := strings.Cut(token, "=")
			if flag != target {
				continue
			}
			switch {
			case hasValue:
				value = inline
			case i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "--"):
				value = fields[i+1]
			}
		}
	}
	return value
}

func withoutScheduledFlags(env string) string {
	lines := strings.Split(env, "\n")
	output := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := launchFields(line)
		if len(fields) > 0 && (fields[0] == "-p" || fields[0] == "--publish") {
			continue
		}
		if isEnvAssignment(line) {
			output = append(output, line)
			continue
		}
		kept := make([]string, 0, len(fields))
		for i := 0; i < len(fields); i++ {
			flag, _, hasValue := strings.Cut(fields[i], "=")
			if scheduledFlag(flag) {
				if !hasValue && i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "--") {
					i++
				}
				continue
			}
			kept = append(kept, fields[i])
		}
		if len(kept) == len(fields) {
			output = append(output, line)
		} else if len(kept) > 0 {
			output = append(output, strings.Join(kept, " "))
		}
	}
	return strings.Join(output, "\n")
}

func launchFields(line string) []string {
	return strings.Fields(strings.TrimSuffix(strings.TrimSpace(line), "\\"))
}

func isEnvAssignment(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Contains(line, "=") && !strings.HasPrefix(line, "--")
}
