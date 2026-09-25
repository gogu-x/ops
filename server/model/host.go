package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Shared sentinel errors returned by repository and actor implementations
// across packages (host/internal, instance/internal, admin/internal/...).
// Centralizing them here lets admin/internal's HTTP handlers distinguish
// error cases (e.g. 404 vs 409) without importing another package's
// internal package, which Go's visibility rules forbid across sibling
// module trees.
var (
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrHostUnresolved       = errors.New("unable to resolve host for service instance")
	ErrHostProjectConflict  = errors.New("host is not bound to this project")
	ErrHostInUse            = errors.New("host is used by service instances")
	ErrHostBound            = errors.New("host is still bound to a project")
	ErrProjectHasBoundHosts = errors.New("project has bound hosts")
)

// PortMapping describes a single container port published to the host,
// using the same semantics as `docker run -p [host_ip:]host_port:container_port[/protocol]`.
type PortMapping struct {
	HostIP        string // optional; empty binds all host interfaces (0.0.0.0)
	HostPort      string
	ContainerPort string
	Protocol      string // "tcp" (default) or "udp"
}

// ContainerSpec describes the parameters required to create and start a
// container for a service instance.
type ContainerSpec struct {
	Name          string
	Image         string
	Cmd           []string
	Env           []string
	Labels        map[string]string
	Ports         []PortMapping
	NetworkMode   string
	RestartPolicy string
}

// OpsInstanceLabel stores the owning service instance ID on the container,
// used to look up/filter containers belonging to a specific instance.
const OpsInstanceLabel = "ops.instance_id"

var instanceNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidateServiceInstance checks that a service instance's fields are
// well-formed before it is persisted or deployed.
func ValidateServiceInstance(item ServiceInstance) error {
	if strings.TrimSpace(item.ServiceTypeID) == "" {
		return errors.New("service_type_id is required")
	}
	if strings.TrimSpace(item.HostID) == "" {
		return errors.New("host_id is required")
	}
	if !instanceNamePattern.MatchString(item.Name) {
		return errors.New("name must contain only lowercase letters, numbers, _ or -")
	}
	if strings.TrimSpace(item.Image) == "" {
		return errors.New("image is required")
	}
	for index, param := range item.Params {
		if !strings.HasPrefix(strings.TrimSpace(param.Flag), "--") {
			return fmt.Errorf("params[%d].flag must start with --", index)
		}
		if strings.TrimSpace(param.Value) == "" {
			return fmt.Errorf("params[%d].value is required", index)
		}
	}
	if strings.TrimSpace(item.PortMapping) != "" {
		if _, ok := ParsePortMappingField(item.PortMapping); !ok {
			return errors.New("port_mapping must look like host_port:container_port, e.g. 9001:9001")
		}
	}
	return nil
}

// ParsePortMappingField parses the dedicated "端口映射" form field, using the
// same "host_port:container_port[/protocol]" syntax as a `-p` value (without
// the flag itself). If only a single port number is given, it is used for
// both host and container ports. Returns ok=false for a blank field.
func ParsePortMappingField(raw string) (PortMapping, bool) {
	spec := strings.TrimSpace(raw)
	if spec == "" {
		return PortMapping{}, false
	}
	protocol := "tcp"
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}
	parts := strings.Split(spec, ":")
	var hostPort, containerPort string
	switch len(parts) {
	case 1:
		hostPort, containerPort = parts[0], parts[0]
	case 2:
		hostPort, containerPort = parts[0], parts[1]
	default:
		return PortMapping{}, false
	}
	hostPort = strings.TrimSpace(hostPort)
	containerPort = strings.TrimSpace(containerPort)
	if hostPort == "" || containerPort == "" {
		return PortMapping{}, false
	}
	return PortMapping{HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol}, true
}

// NewServiceInstance constructs a new ServiceInstance with a generated ID
// and creation/update timestamps.
func NewServiceInstance(serviceTypeID, hostID, name, image, note, envText, network, portMapping string, params []ServiceParam) ServiceInstance {
	now := time.Now().UTC()
	return ServiceInstance{ID: uuid.NewString(), ServiceTypeID: serviceTypeID, HostID: hostID, Name: name, Image: image, Note: note, EnvText: envText, Network: network, PortMapping: portMapping, Params: params, CreatedAt: now, UpdatedAt: now}
}
