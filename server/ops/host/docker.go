package host

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gogu-x/ops/ops/internal/model"
)

// OpsContainerLabel marks containers created by the ops platform so they can
// be reliably discovered and filtered from the rest of the Docker host.
const OpsContainerLabel = "ops.managed"

// OpsInstanceLabel stores the owning service instance ID on the container,
// used to look up/filter containers belonging to a specific instance.
const OpsInstanceLabel = "ops.instance_id"

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

type DockerManager struct {
	mu      sync.Mutex
	clients map[string]*client.Client
}

func NewDockerManager() *DockerManager {
	return &DockerManager{clients: make(map[string]*client.Client)}
}

func (m *DockerManager) Client(host model.Host) (*client.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.clients[host.ID]; existing != nil {
		return existing, nil
	}
	created, err := newClient(host)
	if err != nil {
		return nil, err
	}
	m.clients[host.ID] = created
	return created, nil
}

func (m *DockerManager) Test(ctx context.Context, host model.Host) (map[string]string, error) {
	cli, err := m.Client(host)
	if err != nil {
		return nil, err
	}
	if _, err := cli.Ping(ctx); err != nil {
		return nil, err
	}
	version, err := cli.ServerVersion(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{"version": version.Version, "api_version": version.APIVersion, "os": version.Os, "arch": version.Arch}, nil
}

// ContainerList returns containers managed by the ops platform on the given
// host (identified by the OpsContainerLabel label), including stopped ones.
func (m *DockerManager) ContainerList(ctx context.Context, h model.Host) ([]types.Container, error) {
	cli, err := m.Client(h)
	if err != nil {
		return nil, err
	}
	filterArgs := filters.NewArgs(filters.Arg("label", OpsContainerLabel+"=true"))
	return cli.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
}

// ContainerInspect returns detailed information about a single container.
func (m *DockerManager) ContainerInspect(ctx context.Context, h model.Host, containerID string) (types.ContainerJSON, error) {
	cli, err := m.Client(h)
	if err != nil {
		return types.ContainerJSON{}, err
	}
	return cli.ContainerInspect(ctx, containerID)
}

// ContainerCreate creates (but does not start) a container for the given spec.
func (m *DockerManager) ContainerCreate(ctx context.Context, h model.Host, spec ContainerSpec) (string, error) {
	cli, err := m.Client(h)
	if err != nil {
		return "", err
	}
	labels := map[string]string{OpsContainerLabel: "true"}
	for k, v := range spec.Labels {
		labels[k] = v
	}
	restartPolicy := container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)}
	if spec.RestartPolicy == "" {
		restartPolicy = container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
	}
	exposedPorts, portBindings, err := buildPortBindings(spec.Ports)
	if err != nil {
		return "", err
	}
	hostConfig := &container.HostConfig{RestartPolicy: restartPolicy, PortBindings: portBindings}
	if spec.NetworkMode != "" {
		hostConfig.NetworkMode = container.NetworkMode(spec.NetworkMode)
	}
	config := &container.Config{
		Image:        spec.Image,
		Cmd:          spec.Cmd,
		Env:          spec.Env,
		Labels:       labels,
		ExposedPorts: exposedPorts,
	}
	response, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, spec.Name)
	if err != nil {
		return "", err
	}
	return response.ID, nil
}

// buildPortBindings converts PortMapping entries into the Docker SDK's
// ExposedPorts/PortBindings shapes required by ContainerCreate.
func buildPortBindings(mappings []PortMapping) (nat.PortSet, nat.PortMap, error) {
	if len(mappings) == 0 {
		return nil, nil, nil
	}
	exposedPorts := make(nat.PortSet, len(mappings))
	portBindings := make(nat.PortMap, len(mappings))
	for _, mapping := range mappings {
		protocol := strings.ToLower(strings.TrimSpace(mapping.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		containerPort, err := nat.NewPort(protocol, mapping.ContainerPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid container port %q: %w", mapping.ContainerPort, err)
		}
		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = append(portBindings[containerPort], nat.PortBinding{
			HostIP:   mapping.HostIP,
			HostPort: mapping.HostPort,
		})
	}
	return exposedPorts, portBindings, nil
}

// ContainerStart starts an existing container.
func (m *DockerManager) ContainerStart(ctx context.Context, h model.Host, containerID string) error {
	cli, err := m.Client(h)
	if err != nil {
		return err
	}
	return cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// ContainerStop stops a running container.
func (m *DockerManager) ContainerStop(ctx context.Context, h model.Host, containerID string) error {
	cli, err := m.Client(h)
	if err != nil {
		return err
	}
	return cli.ContainerStop(ctx, containerID, container.StopOptions{})
}

// ContainerRestart restarts a container.
func (m *DockerManager) ContainerRestart(ctx context.Context, h model.Host, containerID string) error {
	cli, err := m.Client(h)
	if err != nil {
		return err
	}
	return cli.ContainerRestart(ctx, containerID, container.StopOptions{})
}

// ContainerRemove force-removes a container (stopping it first if running).
func (m *DockerManager) ContainerRemove(ctx context.Context, h model.Host, containerID string) error {
	cli, err := m.Client(h)
	if err != nil {
		return err
	}
	return cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// ContainerLogs returns the tail of a container's combined stdout/stderr log
// output as plain text (docker's stream framing is stripped by the caller
// when the container was created without a TTY; for simplicity here we
// request logs with Details/Timestamps disabled and a bounded tail).
func (m *DockerManager) ContainerLogs(ctx context.Context, h model.Host, containerID string, tail string) (io.ReadCloser, error) {
	cli, err := m.Client(h)
	if err != nil {
		return nil, err
	}
	if tail == "" {
		tail = "200"
	}
	return cli.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: tail, Timestamps: false})
}

// ImagePull pulls an image (by name:tag) from its registry onto the given
// host, blocking until the pull completes. Docker streams progress as
// newline-delimited JSON; the body is drained and discarded since the ops
// platform only needs the final success/failure.
func (m *DockerManager) ImagePull(ctx context.Context, h model.Host, imageRef string) error {
	cli, err := m.Client(h)
	if err != nil {
		return err
	}
	reader, err := cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("pull image %s: %w", imageRef, err)
	}
	return nil
}

func (m *DockerManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var first error
	for id, cli := range m.clients {
		if err := cli.Close(); err != nil && first == nil {
			first = fmt.Errorf("close docker client %s: %w", id, err)
		}
		delete(m.clients, id)
	}
	return first
}

func newClient(host model.Host) (*client.Client, error) {
	dockerHost := strings.TrimSpace(host.DockerHost)
	if !strings.HasPrefix(dockerHost, "tcp://") {
		return nil, fmt.Errorf("docker_host must use tcp://, got %q", host.DockerHost)
	}
	if !strings.HasSuffix(dockerHost, ":2376") {
		return nil, fmt.Errorf("docker_host must use TCP port 2376, got %q", host.DockerHost)
	}
	httpClient, err := newTLSHTTPClient(host.TLSCA, host.TLSCert, host.TLSKey)
	if err != nil {
		return nil, err
	}
	return client.NewClientWithOpts(
		client.WithHost(dockerHost),
		client.WithHTTPClient(httpClient),
		client.WithAPIVersionNegotiation(),
	)
}

func newTLSHTTPClient(caPEM, certPEM, keyPEM string) (*http.Client, error) {
	if strings.TrimSpace(caPEM) == "" || strings.TrimSpace(certPEM) == "" || strings.TrimSpace(keyPEM) == "" {
		return nil, fmt.Errorf("TLS CA, client certificate, and client key are required")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("tls_ca does not contain a valid PEM certificate")
	}
	tlsConfig.RootCAs = pool
	certificate, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse TLS client certificate/key: %w", err)
	}
	tlsConfig.Certificates = []tls.Certificate{certificate}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}, nil
}
