package internal

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gogu-x/ops/model"
	"github.com/gogu-x/tree"
)

// ErrHostUnresolved, ErrNotFound and ErrAlreadyExists alias the shared model
// sentinels so that admin/internal's HTTP handlers (which cannot import this
// internal package) can distinguish error cases via
// model.ErrHostUnresolved / model.ErrNotFound / model.ErrAlreadyExists,
// while existing call sites within this package keep using the unqualified
// names.
var ErrHostUnresolved = model.ErrHostUnresolved

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

// buildContainerSpec converts a service instance's image, structured params
// and free-form env text into a Docker container spec. Params are appended
// as CLI arguments (flag + value); EnvText lines are parsed as either
// KEY=VALUE env vars, "-p"/"--publish" port mappings (same syntax as `docker
// run -p`), or, if none of those match, additional CLI args. The dedicated
// Network and PortMapping fields are added on top of (not instead of) any
// port mappings found in EnvText.
func buildContainerSpec(item model.ServiceInstance) model.ContainerSpec {
	var cmd []string
	for _, param := range item.Params {
		cmd = append(cmd, param.Flag, param.Value)
	}

	var env []string
	var ports []model.PortMapping
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

	return model.ContainerSpec{
		Name:          containerName(item),
		Image:         item.Image,
		Cmd:           cmd,
		Env:           env,
		Ports:         ports,
		NetworkMode:   strings.TrimSpace(item.Network),
		RestartPolicy: model.NormalizeRestartPolicy(item.RestartPolicy),
		Labels: map[string]string{
			model.OpsInstanceLabel: item.ID,
		},
	}
}

// parsePortMappingField parses the dedicated "端口映射" form field. It
// delegates to model.ParsePortMappingField, kept as a package-level function
// here for backward compatibility with existing call sites/tests.
func parsePortMappingField(raw string) (model.PortMapping, bool) {
	return model.ParsePortMappingField(raw)
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
func parsePortMappingLine(line string) (model.PortMapping, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 || (fields[0] != "-p" && fields[0] != "--publish") {
		return model.PortMapping{}, false
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
		return model.PortMapping{}, false
	}
	if strings.TrimSpace(hostPort) == "" || strings.TrimSpace(containerPort) == "" {
		return model.PortMapping{}, false
	}
	return model.PortMapping{HostIP: hostIP, HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol}, true
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
	if c.Created > 0 {
		item.StartedAt = time.Unix(c.Created, 0).UTC().Format(time.RFC3339)
	}
}

// findContainerForInstance searches ops-managed containers on a host for
// the one matching the given service instance ID label.
func findContainerForInstance(containers []types.Container, instanceID string) (types.Container, bool) {
	for _, c := range containers {
		if c.Labels[model.OpsInstanceLabel] == instanceID {
			return c, true
		}
	}
	return types.Container{}, false
}

func (a *InstanceService) deploy(id string) (model.ServiceInstance, error) {
	return a.deployWithEvent(id, true)
}

// deployWithEvent implements deploy, optionally suppressing the
// deploy_succeeded/deploy_failed event (the "restart" container action
// calls this internally via redeploy and records its own
// container_restarted/container_restart_failed event instead).
func (a *InstanceService) deployWithEvent(id string, recordDeployEvent bool) (model.ServiceInstance, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return model.ServiceInstance{}, ErrHostUnresolved
	}
	result, err := a.redeploy(item, item.HostID)
	if !recordDeployEvent {
		return result, err
	}
	if err != nil {
		a.recordEvent(id, model.EventDeployFailed, err.Error())
	} else {
		a.recordEvent(id, model.EventDeploySucceeded, fmt.Sprintf("镜像 %s 部署成功", result.Image))
	}
	return result, err
}

// updateImage changes a service instance's image tag, persists it, pulls
// the new image onto the instance's host, and redeploys the container so
// it picks up the new version. If the pull or redeploy fails, the image
// change is still persisted (so retrying "更新镜像" doesn't require
// re-entering the tag), but the instance is left in an error state.
func (a *InstanceService) updateImage(id, newImage string) (model.ServiceInstance, error) {
	newImage = strings.TrimSpace(newImage)
	if newImage == "" {
		return model.ServiceInstance{}, errors.New("image is required")
	}
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return model.ServiceInstance{}, ErrHostUnresolved
	}
	hostID := item.HostID
	item.Image = newImage
	item.UpdatedAt = time.Now().UTC()
	if err := a.repo.Update(bgCtx(), item); err != nil {
		return model.ServiceInstance{}, err
	}
	if _, err := hostRequest(model.ImagePullRequest{HostID: hostID, Image: newImage}); err != nil {
		item.Status = model.StatusError
		item.DeployError = fmt.Sprintf("pull image: %s", err.Error())
		a.recordEvent(id, model.EventUpdateImageFailed, fmt.Sprintf("拉取镜像 %s 失败: %s", newImage, err.Error()))
		return item, fmt.Errorf("pull image %s: %w", newImage, err)
	}
	result, err := a.redeploy(item, hostID)
	if err != nil {
		a.recordEvent(id, model.EventUpdateImageFailed, err.Error())
	} else {
		a.recordEvent(id, model.EventUpdateImageSucceeded, fmt.Sprintf("镜像已更新为 %s 并重新部署", newImage))
	}
	return result, err
}

// redeploy removes any existing container for the instance (matched by the
// ops.instance_id label, not by name) and creates a fresh one from the
// instance's current configuration. Used by both DeployRequest and
// UpdateImageRequest so redeploying after an edit, a failed deploy, or an
// image update never hits a "name already in use" conflict.
func (a *InstanceService) redeploy(item model.ServiceInstance, hostID string) (model.ServiceInstance, error) {
	if existingID, err := a.lookupContainerID(hostID, item); err == nil {
		if _, err := hostRequest(model.ContainerActionRequest{HostID: hostID, ContainerID: existingID, Action: "remove"}); err != nil {
			item.Status = model.StatusError
			item.DeployError = fmt.Sprintf("remove previous container: %s", err.Error())
			return item, fmt.Errorf("remove previous container: %w", err)
		}
	}
	spec := buildContainerSpec(item)
	value, err := hostRequest(model.ContainerDeployRequest{HostID: hostID, Spec: spec})
	if err != nil {
		item.Status = model.StatusError
		item.DeployError = err.Error()
		return item, fmt.Errorf("deploy container: %w", err)
	}
	result, ok := value.(model.ContainerDeployResponse)
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
func (a *InstanceService) containerAction(id, action string) error {
	if action == "restart" {
		_, err := a.deployWithEvent(id, false)
		if err != nil {
			a.recordEvent(id, model.EventContainerRestartFailed, err.Error())
		} else {
			a.recordEvent(id, model.EventContainerRestarted, "容器已重启")
		}
		return err
	}
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return ErrHostUnresolved
	}
	containerID, err := a.lookupContainerID(item.HostID, item)
	if err != nil {
		return err
	}
	_, err = hostRequest(model.ContainerActionRequest{HostID: item.HostID, ContainerID: containerID, Action: action})
	a.recordContainerActionEvent(id, action, err)
	return err
}

// recordContainerActionEvent records a lifecycle event for a start/stop
// action (restart is recorded by deploy, since it redeploys).
func (a *InstanceService) recordContainerActionEvent(id, action string, err error) {
	var okType, failType, okMsg string
	switch action {
	case "start":
		okType, failType, okMsg = model.EventContainerStarted, model.EventContainerStartFailed, "容器启动成功"
	case "stop":
		okType, failType, okMsg = model.EventContainerStopped, model.EventContainerStopFailed, "容器已停止"
	default:
		return
	}
	if err != nil {
		a.recordEvent(id, failType, err.Error())
		return
	}
	a.recordEvent(id, okType, okMsg)
}

func (a *InstanceService) removeContainer(id string) error {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return ErrHostUnresolved
	}
	containerID, err := a.lookupContainerID(item.HostID, item)
	if err != nil {
		return err
	}
	_, err = hostRequest(model.ContainerActionRequest{HostID: item.HostID, ContainerID: containerID, Action: "remove"})
	if err != nil {
		a.recordEvent(id, model.EventContainerRemoveFailed, err.Error())
	} else {
		a.recordEvent(id, model.EventContainerRemoved, "容器已移除")
	}
	return err
}

func (a *InstanceService) status(id string) (model.ServiceInstance, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ServiceInstance{}, err
	}
	if strings.TrimSpace(item.HostID) == "" {
		item.Status = model.StatusUnknown
		return item, nil
	}
	value, err := hostRequest(model.ContainerListRequest{HostID: item.HostID})
	if err != nil {
		item.Status = model.StatusUnknown
		return item, nil
	}
	result, ok := value.(model.ContainerListResponse)
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

func (a *InstanceService) detail(id string) (model.ContainerDetail, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return model.ContainerDetail{}, err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return model.ContainerDetail{}, ErrHostUnresolved
	}
	containerID, err := a.lookupContainerID(item.HostID, item)
	if err != nil {
		return model.ContainerDetail{}, err
	}
	value, err := hostRequest(model.ContainerInspectRequest{HostID: item.HostID, ContainerID: containerID})
	if err != nil {
		return model.ContainerDetail{}, err
	}
	result, ok := value.(model.ContainerInspectResponse)
	if !ok {
		return model.ContainerDetail{}, errors.New("invalid host actor response")
	}
	return buildContainerDetail(result.Container), nil
}

// buildContainerDetail projects a full Docker inspect response into the
// JSON-friendly ContainerDetail shape exposed to the frontend.
func buildContainerDetail(c types.ContainerJSON) model.ContainerDetail {
	detail := model.ContainerDetail{
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

func (a *InstanceService) logs(id, tail string) (string, error) {
	item, err := a.repo.Get(bgCtx(), id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(item.HostID) == "" {
		return "", ErrHostUnresolved
	}
	containerID, err := a.lookupContainerID(item.HostID, item)
	if err != nil {
		return "", err
	}
	value, err := hostRequest(model.ContainerLogsRequest{HostID: item.HostID, ContainerID: containerID, Tail: tail})
	if err != nil {
		return "", err
	}
	result, ok := value.(model.ContainerLogsResponse)
	if !ok {
		return "", errors.New("invalid host actor response")
	}
	return result.Logs, nil
}

// listEvents returns recent lifecycle events for a service instance,
// most-recent first.
func (a *InstanceService) listEvents(id string, limit int) ([]model.InstanceEvent, error) {
	if a.events == nil {
		return []model.InstanceEvent{}, nil
	}
	events, err := a.events.ListByInstance(bgCtx(), id, limit)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []model.InstanceEvent{}
	}
	return events, nil
}

// lookupContainerID finds the current container ID for an instance by
// querying the host's ops-managed containers (the ServiceInstance itself
// does not persist container_id).
func (a *InstanceService) lookupContainerID(hostID string, item model.ServiceInstance) (string, error) {
	value, err := hostRequest(model.ContainerListRequest{HostID: hostID})
	if err != nil {
		return "", err
	}
	result, ok := value.(model.ContainerListResponse)
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
func (a *InstanceService) enrichList(items []model.ServiceInstance) []model.ServiceInstance {
	hostContainers := make(map[string][]types.Container)
	for i := range items {
		item := &items[i]
		hostID := strings.TrimSpace(item.HostID)
		if hostID == "" {
			item.Status = model.StatusUnknown
			continue
		}
		containers, ok := hostContainers[hostID]
		if !ok {
			value, err := hostRequest(model.ContainerListRequest{HostID: hostID})
			if err != nil {
				item.Status = model.StatusUnknown
				hostContainers[hostID] = nil
				continue
			}
			result, ok := value.(model.ContainerListResponse)
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
