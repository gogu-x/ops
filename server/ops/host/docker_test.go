package host

import (
	"strings"
	"testing"

	"github.com/gogu-x/ops/ops/internal/model"
)

func TestDockerClientRequiresTCP2376TLS(t *testing.T) {
	_, err := newClient(model.Host{DockerHost: "tcp://127.0.0.1:2375", TLSCA: "ca", TLSCert: "cert", TLSKey: "key"})
	if err == nil || !strings.Contains(err.Error(), "2376") {
		t.Fatalf("expected 2376 validation error, got %v", err)
	}
	_, err = newClient(model.Host{DockerHost: "tcp://127.0.0.1:2376"})
	if err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("expected TLS credential error, got %v", err)
	}
}
