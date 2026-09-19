package instance

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/tree"
)

var ErrHostUnresolved = errors.New("unable to resolve host for service instance")

// DeployRequest creates and starts a Docker container for a service
// instance. It is safe to call again after a previous deploy failed; any
// existing container with the same name is not implicitly reused.
type DeployRequest struct{ ID string }

// UpdateImageRequest updates a service instance's image (e.g. to a new
// version tag), pulls the new image on the instance's host, and redeploys
// the container so it runs the updated image.
type UpdateImageRequest struct {
	ID    string
	Image string
}

// ContainerActionRequest performs start/stop/restart on an already deployed
// service instance's container.
type ContainerActionRequest struct {
	ID     string
	Action string // "start", "stop", "restart"
}

// RemoveContainerRequest stops (if needed) and removes the container behind
// a service instance, without deleting the instance's metadata.
type RemoveContainerRequest struct{ ID string }

// StatusRequest returns the live container status for a single instance.
type StatusRequest struct{ ID string }

// DetailRequest returns full Docker inspect information for a deployed
// service instance's container.
type DetailRequest struct{ ID string }

// ContainerDetail is a JSON-friendly projection of the Docker inspect
// response, exposing the fields useful for troubleshooting in the UI.
type ContainerDetail struct {
	ContainerID   string            `json:"container_id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	Status        string            `json:"status"`
	State         string            `json:"state"`
	Running       bool              `json:"running"`
	RestartCount  int               `json:"restart_count"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	ExitCode      int               `json:"exit_code"`
	Error         string            `json:"error,omitempty"`
	Cmd           []string          `json:"cmd"`
	Env           []string          `json:"env"`
	Labels        map[string]string `json:"labels"`
	NetworkMode   string            `json:"network_mode"`
	RestartPolicy string            `json:"restart_policy"`
	IPAddress     string            `json:"ip_address"`
	Ports         []string          `json:"ports"`
	Mounts        []string          `json:"mounts"`
	Created       string            `json:"created"`
}
type DetailResponse struct{ Detail ContainerDetail }

// LogsRequest returns recent log output for a service instance's container.
type LogsRequest struct {
	ID   string
	Tail string
}
type LogsResponse struct{ Logs string }

// containerName derives the Docker container name from the service
// instance's user-facing name (e.g. "game-1"), so the name shown by `docker
// ps` matches what the client configured in the ops UI.
func containerName(item model.ServiceInstance) string {
	return item.Name
}

func hostRequest(message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup("ops-host")
	if !ok {
		return nil, errors.New("host actor is unavailable")
	}
	return tree.Request(pid, message).AwaitTimeout(20 * time.Second)
}

// resolveHostID looks up the Docker host backing a service instance via its
// ServiceType.
func (a *Actor) resolveHostID(serviceTypeID string) (string, error) {
	if a.serviceTypes == nil {
		return "", errors.New("service type repository is unavailable")
	}
	items, err := a.serviceTypes.List(bgCtx())
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.ID == serviceTypeID {
			if strings.TrimSpace(item.HostID) == "" {
				return "", ErrHostUnresolved
			}
			return item.HostID, nil
		}
	}
	return "", ErrHostUnresolved
}

// buildContainerSpec converts a service instance's image, structured params
// and free-form env text into a Docker container spec. Params are appended
// as CLI arguments (flag + value); EnvText lines are parsed as either
// KEY=VALUE env vars, "-p"/"--publish" port mappings (same syntax as `docker
// run -p`), or, if none of those match, additional CLI args. The dedicated
// Network and PortMapping fields are added on top of (not instead of) any
// port mappings found in EnvText.
func buildContainerSpec(item model.ServiceInstance) host.ContainerSpec {
	var cmd []string
	for _, param := range item.Params {
		cmd = append(cmd, param.Flag, param.Value)
	}

	var env []string
	var ports []host.PortMapping
	scanner := bufio.NewScanner(strings.NewReader(item.EnvText))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if mapping, ok := parsePortMappingLine(line); ok {
			ports = append(ports, mapping)
			continue
		}
		if strings.Contains(line, "=") && !strings.HasPrefix(line, "--") {
			env = append(env, line)
			continue
		}
		// treat as extra CLI args, e.g. "--etcd etcd:2379"
		fields := strings.Fields(line)
		cmd = append(cmd, fields...)
	}

	if mapping, ok := parsePortMappingField(item.PortMapping); ok {
		ports = append(ports, mapping)
	}

	return host.ContainerSpec{
		Name:        containerName(item),
		Image:       item.Image,
		Cmd:         cmd,
		Env:         env,
		Ports:       ports,
		NetworkMode: strings.TrimSpace(item.Network),
		Labels: map[string]string{
			host.OpsInstanceLabel: item.ID,
		},
	}
}

// parsePortMappingField parses the dedicated "端口映射" form field, using the
// same "host_port:container_port[/protocol]" syntax as a `-p` value (without
// the flag itself). If only a single port number is given, it is used for
// both host and container ports. Returns ok=false for a blank field.
func parsePortMappingField(raw string) (host.PortMapping, bool) {
	spec := strings.TrimSpace(raw)
	if spec == "" {
		return host.PortMapping{}, false
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
		return host.PortMapping{}, false
	}
	hostPort = strings.TrimSpace(hostPort)
	containerPort = strings.TrimSpace(containerPort)
	if hostPort == "" || containerPort == "" {
		return host.PortMapping{}, false
	}
	return host.PortMapping{HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol}, true
}

// parsePortMappingLine recognizes "-p"/"--publish" lines using the same
// syntax as `docker run -p`, e.g.:
//
//	-p 9001:9001
//	-p 9001:9001/udp
//	--publish 0.0.0.0:9001:9001
//
// It returns ok=false for any line that isn't a port publish directive so
// the caller can fall back to treating it as env or extra CLI args.
func parsePortMappingLine(line string) (host.PortMapping, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 || (fields[0] != "-p" && fields[0] != "--publish") {
		return host.PortMapping{}, false
	}
	spec := fields[1]
	protocol := "tcp"
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}
	parts := strings.Split(spec, ":")
	var hostIP, hostPort, containerPort string
	switch len(parts) {
	case 2:
		hostPort, containerPort = parts[0], parts[1]
	case 3:
		hostIP, hostPort, containerPort = parts[0], parts[1], parts[2]
	default:
		return host.PortMapping{}, false
	}
	if strings.TrimSpace(hostPort) == "" || strings.TrimSpace(containerPort) == "" {
		return host.PortMapping{}, false
	}
	return host.PortMapping{HostIP: hostIP, HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol}, true
}

// mapContainerStatus converts Docker's container state string into the
// ServiceInstance status enum exposed to the frontend.
func mapContainerStatus(state string) string {
	switch state {
	case "running":
		return model.StatusRunning
	case "restarting":
		return model.StatusRestarting
	case "exited", "created", "paused", "dead", "removing":
		return model.StatusStopped
	case "":
		return model.StatusNotDeployed
	default:
		return model.StatusUnknown
	}
}

// applyContainerSummary enriches a service instance with runtime fields
// derived from a Docker container list entry.
func applyContainerSummary(item *model.ServiceInstance, c types.Container) {
	item.ContainerID = c.ID
	item.Status = mapContainerStatus(c.State)
	item.StatusText = c.Status
	item.ContainerImage = c.Image
}

// findContainerForInstance searches ops-managed containers on a host for
// the one matching the given service instance ID label.
func findContainerForInstance(containers []types.Container, instanceID string) (types.Container, bool) {
	for _, c := range containers {
		if c.Labels[host.OpsInstanceLabel] == instanceID {
			return c, true
		}
	}
	return types.Container{}, false
}

func (a *Actor) deploy(id string) (model.ServiceInstance, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	return a.redeploy(item, hostID)
}

// updateImage changes a service instance's image tag, persists it, pulls
// the new image onto the instance's host, and redeploys the container so
// it picks up the new version. If the pull or redeploy fails, the image
// change is still persisted (so retrying "更新镜像" doesn't require
// re-entering the tag), but the instance is left in an error state.
func (a *Actor) updateImage(id, newImage string) (model.ServiceInstance, error) {
	newImage = strings.TrimSpace(newImage)
	if newImage == "" {
		return model.ServiceInstance{}, errors.New("image is required")
	}
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	item.Image = newImage
	item.UpdatedAt = time.Now().UTC()
	if err := a.repo.Update(bgCtx(), item); err != nil {
		return model.ServiceInstance{}, err
	}
	if _, err := hostRequest(host.ImagePullRequest{HostID: hostID, Image: newImage}); err != nil {
		item.Status = model.StatusError
		item.DeployError = fmt.Sprintf("pull image: %s", err.Error())
		return item, fmt.Errorf("pull image %s: %w", newImage, err)
	}
	return a.redeploy(item, hostID)
}

// redeploy removes any existing container for the instance (matched by the
// ops.instance_id label, not by name) and creates a fresh one from the
// instance's current configuration. Used by both DeployRequest and
// UpdateImageRequest so redeploying after an edit, a failed deploy, or an
// image update never hits a "name already in use" conflict.
func (a *Actor) redeploy(item model.ServiceInstance, hostID string) (model.ServiceInstance, error) {
	if existingID, err := a.lookupContainerID(hostID, item); err == nil {
		if _, err := hostRequest(host.ContainerActionRequest{HostID: hostID, ContainerID: existingID, Action: "remove"}); err != nil {
			item.Status = model.StatusError
			item.DeployError = fmt.Sprintf("remove previous container: %s", err.Error())
			return item, fmt.Errorf("remove previous container: %w", err)
		}
	}
	spec := buildContainerSpec(item)
	value, err := hostRequest(host.ContainerDeployRequest{HostID: hostID, Spec: spec})
	if err != nil {
		item.Status = model.StatusError
		item.DeployError = err.Error()
		return item, fmt.Errorf("deploy container: %w", err)
	}
	result, ok := value.(host.ContainerDeployResponse)
	if !ok {
		return model.ServiceInstance{}, errors.New("invalid host actor response")
	}
	item.ContainerID = result.ContainerID
	item.Status = model.StatusRunning
	item.ContainerImage = item.Image
	item.DeployError = ""
	return item, nil
}

// containerAction performs start/stop on an existing container as-is. For
// "restart" it intentionally redeploys (removes and recreates) the
// container instead of issuing a bare `docker restart`, because a bare
// restart reuses the container's original Cmd/Env/Ports baked in at
// creation time and would silently ignore any config changes (image,
// params, env text, port mappings) made since the last deploy.
func (a *Actor) containerAction(id, action string) error {
	if action == "restart" {
		_, err := a.deploy(id)
		return err
	}
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return err
	}
	containerID, err := a.lookupContainerID(hostID, item)
	if err != nil {
		return err
	}
	_, err = hostRequest(host.ContainerActionRequest{HostID: hostID, ContainerID: containerID, Action: action})
	return err
}

func (a *Actor) removeContainer(id string) error {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return err
	}
	containerID, err := a.lookupContainerID(hostID, item)
	if err != nil {
		return err
	}
	_, err = hostRequest(host.ContainerActionRequest{HostID: hostID, ContainerID: containerID, Action: "remove"})
	return err
}

func (a *Actor) status(id string) (model.ServiceInstance, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		item.Status = model.StatusUnknown
		return item, nil
	}
	value, err := hostRequest(host.ContainerListRequest{HostID: hostID})
	if err != nil {
		item.Status = model.StatusUnknown
		return item, nil
	}
	result, ok := value.(host.ContainerListResponse)
	if !ok {
		item.Status = model.StatusUnknown
		return item, nil
	}
	if c, found := findContainerForInstance(result.Containers, item.ID); found {
		applyContainerSummary(&item, c)
	} else {
		item.Status = model.StatusNotDeployed
	}
	return item, nil
}

func (a *Actor) detail(id string) (ContainerDetail, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return ContainerDetail{}, err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return ContainerDetail{}, err
	}
	containerID, err := a.lookupContainerID(hostID, item)
	if err != nil {
		return ContainerDetail{}, err
	}
	value, err := hostRequest(host.ContainerInspectRequest{HostID: hostID, ContainerID: containerID})
	if err != nil {
		return ContainerDetail{}, err
	}
	result, ok := value.(host.ContainerInspectResponse)
	if !ok {
		return ContainerDetail{}, errors.New("invalid host actor response")
	}
	return buildContainerDetail(result.Container), nil
}

// buildContainerDetail projects a full Docker inspect response into the
// JSON-friendly ContainerDetail shape exposed to the frontend.
func buildContainerDetail(c types.ContainerJSON) ContainerDetail {
	detail := ContainerDetail{
		Name:   strings.TrimPrefix(c.Name, "/"),
		Labels: map[string]string{},
	}
	if c.ContainerJSONBase != nil {
		detail.ContainerID = c.ContainerJSONBase.ID
		detail.Image = c.ContainerJSONBase.Image
		detail.RestartCount = c.ContainerJSONBase.RestartCount
		detail.Created = c.ContainerJSONBase.Created
		if c.ContainerJSONBase.HostConfig != nil {
			detail.NetworkMode = string(c.ContainerJSONBase.HostConfig.NetworkMode)
			detail.RestartPolicy = string(c.ContainerJSONBase.HostConfig.RestartPolicy.Name)
		}
		if c.ContainerJSONBase.State != nil {
			detail.State = c.ContainerJSONBase.State.Status
			detail.Running = c.ContainerJSONBase.State.Running
			detail.StartedAt = c.ContainerJSONBase.State.StartedAt
			detail.FinishedAt = c.ContainerJSONBase.State.FinishedAt
			detail.ExitCode = c.ContainerJSONBase.State.ExitCode
			detail.Error = c.ContainerJSONBase.State.Error
		}
	}
	detail.Status = mapContainerStatus(detail.State)
	if c.Config != nil {
		detail.Cmd = c.Config.Cmd
		detail.Env = c.Config.Env
		if c.Config.Labels != nil {
			detail.Labels = c.Config.Labels
		}
	}
	if c.NetworkSettings != nil {
		detail.IPAddress = c.NetworkSettings.IPAddress
		if detail.IPAddress == "" {
			for _, net := range c.NetworkSettings.Networks {
				if net != nil && net.IPAddress != "" {
					detail.IPAddress = net.IPAddress
					break
				}
			}
		}
		for port, bindings := range c.NetworkSettings.Ports {
			if len(bindings) == 0 {
				detail.Ports = append(detail.Ports, string(port))
				continue
			}
			for _, binding := range bindings {
				detail.Ports = append(detail.Ports, fmt.Sprintf("%s:%s -> %s", binding.HostIP, binding.HostPort, port))
			}
		}
	}
	for _, mount := range c.Mounts {
		detail.Mounts = append(detail.Mounts, fmt.Sprintf("%s -> %s (%s)", mount.Source, mount.Destination, mount.Mode))
	}
	// Normalize nil slices to empty ones so the JSON response always sends
	// "[]" instead of "null" for these fields; frontend code relies on
	// array methods (e.g. .length) without null-checking.
	if detail.Cmd == nil {
		detail.Cmd = []string{}
	}
	if detail.Env == nil {
		detail.Env = []string{}
	}
	if detail.Ports == nil {
		detail.Ports = []string{}
	}
	if detail.Mounts == nil {
		detail.Mounts = []string{}
	}
	return detail
}

func (a *Actor) logs(id, tail string) (string, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return "", err
	}
	hostID, err := a.resolveHostID(item.ServiceTypeID)
	if err != nil {
		return "", err
	}
	containerID, err := a.lookupContainerID(hostID, item)
	if err != nil {
		return "", err
	}
	value, err := hostRequest(host.ContainerLogsRequest{HostID: hostID, ContainerID: containerID, Tail: tail})
	if err != nil {
		return "", err
	}
	result, ok := value.(host.ContainerLogsResponse)
	if !ok {
		return "", errors.New("invalid host actor response")
	}
	return result.Logs, nil
}

// lookupContainerID finds the current container ID for an instance by
// querying the host's ops-managed containers (the ServiceInstance itself
// does not persist container_id).
func (a *Actor) lookupContainerID(hostID string, item model.ServiceInstance) (string, error) {
	value, err := hostRequest(host.ContainerListRequest{HostID: hostID})
	if err != nil {
		return "", err
	}
	result, ok := value.(host.ContainerListResponse)
	if !ok {
		return "", errors.New("invalid host actor response")
	}
	c, found := findContainerForInstance(result.Containers, item.ID)
	if !found {
		return "", errors.New("container not deployed for this instance")
	}
	return c.ID, nil
}

// enrichList annotates a list of service instances with their live
// container status, grouped by host to minimize Docker API calls.
func (a *Actor) enrichList(items []model.ServiceInstance) []model.ServiceInstance {
	typeToHost := make(map[string]string)
	hostContainers := make(map[string][]types.Container)
	for i := range items {
		item := &items[i]
		hostID, ok := typeToHost[item.ServiceTypeID]
		if !ok {
			resolved, err := a.resolveHostID(item.ServiceTypeID)
			if err != nil {
				item.Status = model.StatusUnknown
				typeToHost[item.ServiceTypeID] = ""
				continue
			}
			hostID = resolved
			typeToHost[item.ServiceTypeID] = hostID
		}
		if hostID == "" {
			item.Status = model.StatusUnknown
			continue
		}
		containers, ok := hostContainers[hostID]
		if !ok {
			value, err := hostRequest(host.ContainerListRequest{HostID: hostID})
			if err != nil {
				item.Status = model.StatusUnknown
				hostContainers[hostID] = nil
				continue
			}
			result, ok := value.(host.ContainerListResponse)
			if !ok {
				item.Status = model.StatusUnknown
				hostContainers[hostID] = nil
				continue
			}
			containers = result.Containers
			hostContainers[hostID] = containers
		}
		if c, found := findContainerForInstance(containers, item.ID); found {
			applyContainerSummary(item, c)
		} else {
			item.Status = model.StatusNotDeployed
		}
	}
	return items
}
