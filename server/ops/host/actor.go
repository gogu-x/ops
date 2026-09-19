package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
	"github.com/gogu-x/tree"
)

type ListRequest struct{}
type CreateRequest struct{ Host model.Host }
type DeleteRequest struct{ ID string }
type TestRequest struct{ ID string }

type ListResponse struct{ Hosts []model.Host }
type TestResponse struct {
	Host   model.Host        `json:"host"`
	Docker map[string]string `json:"docker"`
}

// ContainerListRequest lists ops-managed containers on a given host.
type ContainerListRequest struct{ HostID string }
type ContainerListResponse struct{ Containers []types.Container }

// ContainerInspectRequest fetches full details for a single container on a host.
type ContainerInspectRequest struct {
	HostID      string
	ContainerID string
}
type ContainerInspectResponse struct{ Container types.ContainerJSON }

// ContainerDeployRequest creates and starts a new container on a host.
type ContainerDeployRequest struct {
	HostID string
	Spec   ContainerSpec
}
type ContainerDeployResponse struct{ ContainerID string }

// ContainerActionRequest performs start/stop/restart/remove on an existing container.
type ContainerActionRequest struct {
	HostID      string
	ContainerID string
	Action      string // "start", "stop", "restart", "remove"
}

// ContainerLogsRequest fetches recent log output for a container.
type ContainerLogsRequest struct {
	HostID      string
	ContainerID string
	Tail        string
}
type ContainerLogsResponse struct{ Logs string }

// ImagePullRequest pulls an image onto a host from its registry.
type ImagePullRequest struct {
	HostID string
	Image  string
}

type Actor struct {
	repo   Repository
	docker *DockerManager
}

func NewActor(_ conf.Config, repositories ...Repository) *Actor {
	repo := Repository(NewMemoryRepository())
	if len(repositories) > 0 && repositories[0] != nil {
		repo = repositories[0]
	}
	return &Actor{repo: repo, docker: NewDockerManager()}
}

func (a *Actor) Name() string { return "ops-host" }

func (a *Actor) OnInit(_ tree.Context) {}

func (a *Actor) HandleMessage(ctx tree.Context, message interface{}) {
	switch request := message.(type) {
	case ListRequest:
		hosts, err := a.repo.List(context.Background())
		for i := range hosts {
			hosts[i] = publicHost(hosts[i])
		}
		ctx.Response(ListResponse{Hosts: hosts}, err)
	case CreateRequest:
		created, err := a.create(request.Host)
		ctx.Response(publicHost(created), err)
	case DeleteRequest:
		err := a.delete(request.ID)
		ctx.Response(nil, err)
	case TestRequest:
		result, err := a.test(request.ID)
		ctx.Response(result, err)
	case ContainerListRequest:
		result, err := a.containerList(request.HostID)
		ctx.Response(result, err)
	case ContainerInspectRequest:
		result, err := a.containerInspect(request.HostID, request.ContainerID)
		ctx.Response(result, err)
	case ContainerDeployRequest:
		result, err := a.containerDeploy(request.HostID, request.Spec)
		ctx.Response(result, err)
	case ContainerActionRequest:
		err := a.containerAction(request.HostID, request.ContainerID, request.Action)
		ctx.Response(nil, err)
	case ContainerLogsRequest:
		result, err := a.containerLogs(request.HostID, request.ContainerID, request.Tail)
		ctx.Response(result, err)
	case ImagePullRequest:
		err := a.imagePull(request.HostID, request.Image)
		ctx.Response(nil, err)
	default:
		ctx.Response(nil, fmt.Errorf("unsupported host message %T", message))
	}
}

func (a *Actor) OnStop(_ tree.Context) {
	_ = a.docker.Close()
}

func (a *Actor) create(host model.Host) (model.Host, error) {
	if host.Name == "" {
		return model.Host{}, errors.New("host name is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(host.DockerHost), "tcp://") {
		return model.Host{}, errors.New("docker_host must use tcp://")
	}
	if !strings.HasSuffix(strings.TrimSpace(host.DockerHost), ":2376") {
		return model.Host{}, errors.New("docker_host must use TCP port 2376")
	}
	if strings.TrimSpace(host.TLSCA) == "" || strings.TrimSpace(host.TLSCert) == "" || strings.TrimSpace(host.TLSKey) == "" {
		// TLS material is optional: Docker TLS remains enabled, while the
		// client skips server certificate verification when CA is empty.
	}
	created := NewHost(host.Name, host.DockerHost, host.TLSCA, host.TLSCert, host.TLSKey, host.Note)
	host.ID = created.ID
	host.CreatedAt = created.CreatedAt
	host.UpdatedAt = created.UpdatedAt
	if err := a.repo.Create(context.Background(), host); err != nil {
		return model.Host{}, err
	}
	return host, nil
}

func (a *Actor) delete(id string) error {
	if _, err := a.repo.Get(context.Background(), id); err != nil {
		return err
	}
	return a.repo.Delete(context.Background(), id)
}

func (a *Actor) test(id string) (TestResponse, error) {
	host, err := a.repo.Get(context.Background(), id)
	if err != nil {
		return TestResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := a.docker.Test(ctx, host)
	if err != nil {
		return TestResponse{}, err
	}
	return TestResponse{Host: publicHost(host), Docker: info}, nil
}

func publicHost(host model.Host) model.Host {
	if host.TLSCA != "" {
		host.TLSCA = "[已配置]"
	}
	if host.TLSCert != "" {
		host.TLSCert = "[已配置]"
	}
	if host.TLSKey != "" {
		host.TLSKey = "[已配置]"
	}
	return host
}

func (a *Actor) containerList(hostID string) (ContainerListResponse, error) {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return ContainerListResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	items, err := a.docker.ContainerList(ctx, h)
	if err != nil {
		return ContainerListResponse{}, err
	}
	return ContainerListResponse{Containers: items}, nil
}

func (a *Actor) containerInspect(hostID, containerID string) (ContainerInspectResponse, error) {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return ContainerInspectResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	item, err := a.docker.ContainerInspect(ctx, h, containerID)
	if err != nil {
		return ContainerInspectResponse{}, err
	}
	return ContainerInspectResponse{Container: item}, nil
}

func (a *Actor) containerDeploy(hostID string, spec ContainerSpec) (ContainerDeployResponse, error) {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return ContainerDeployResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	containerID, err := a.docker.ContainerCreate(ctx, h, spec)
	if err != nil {
		return ContainerDeployResponse{}, err
	}
	if err := a.docker.ContainerStart(ctx, h, containerID); err != nil {
		return ContainerDeployResponse{}, err
	}
	return ContainerDeployResponse{ContainerID: containerID}, nil
}

func (a *Actor) containerAction(hostID, containerID, action string) error {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch action {
	case "start":
		return a.docker.ContainerStart(ctx, h, containerID)
	case "stop":
		return a.docker.ContainerStop(ctx, h, containerID)
	case "restart":
		return a.docker.ContainerRestart(ctx, h, containerID)
	case "remove":
		return a.docker.ContainerRemove(ctx, h, containerID)
	default:
		return fmt.Errorf("unsupported container action %q", action)
	}
}

func (a *Actor) containerLogs(hostID, containerID, tail string) (ContainerLogsResponse, error) {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return ContainerLogsResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	reader, err := a.docker.ContainerLogs(ctx, h, containerID, tail)
	if err != nil {
		return ContainerLogsResponse{}, err
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return ContainerLogsResponse{}, err
	}
	return ContainerLogsResponse{Logs: demuxDockerLogs(raw)}, nil
}

func (a *Actor) imagePull(hostID, imageRef string) error {
	h, err := a.repo.Get(context.Background(), hostID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return a.docker.ImagePull(ctx, h, imageRef)
}

// demuxDockerLogs strips Docker's multiplexed stdout/stderr frame headers
// (8-byte header per frame) when the container was created without a TTY.
// If the payload does not look framed, it is returned as-is.
func demuxDockerLogs(raw []byte) string {
	var out strings.Builder
	i := 0
	for i+8 <= len(raw) {
		streamType := raw[i]
		if streamType > 2 {
			// Not a recognizable frame header; treat remaining bytes as plain text.
			out.Write(raw[i:])
			return out.String()
		}
		size := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
		start := i + 8
		end := start + size
		if size < 0 || end > len(raw) {
			out.Write(raw[i:])
			return out.String()
		}
		out.Write(raw[start:end])
		i = end
	}
	if i < len(raw) {
		out.Write(raw[i:])
	}
	return out.String()
}
