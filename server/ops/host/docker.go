package host

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/docker/docker/client"
	"github.com/gogu-x/ops/ops/internal/model"
)

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
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(caPEM) == "" {
		// User explicitly allowed empty certificate material. Keep TLS enabled,
		// but do not verify the remote server certificate in this mode.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // explicit optional verification mode
	} else {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, fmt.Errorf("tls_ca does not contain a valid PEM certificate")
		}
		tlsConfig.RootCAs = pool
	}
	if strings.TrimSpace(certPEM) != "" || strings.TrimSpace(keyPEM) != "" {
		if strings.TrimSpace(certPEM) == "" || strings.TrimSpace(keyPEM) == "" {
			return nil, fmt.Errorf("tls_cert and tls_key must be provided together")
		}
		certificate, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("parse TLS client certificate/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}, nil
}
