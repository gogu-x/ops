package host

import (
	"context"
	"errors"
	"testing"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal/model"
)

func tlsHost(name string) model.Host {
	return model.Host{Name: name, DockerHost: "tcp://127.0.0.1:2376", TLSCA: "ca.pem", TLSCert: "client-cert.pem", TLSKey: "client-key.pem"}
}

func TestHostConfigurationLifecycle(t *testing.T) {
	a := NewActor(conf.Config{})
	created, err := a.create(tlsHost("docker-1"))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.DockerHost != "tcp://127.0.0.1:2376" {
		t.Fatalf("unexpected host: %+v", created)
	}
	if _, err := a.create(tlsHost("docker-1")); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := a.delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.repo.Get(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted host, got %v", err)
	}
}

func TestHostTLSValidation(t *testing.T) {
	a := NewActor(conf.Config{})
	if _, err := a.create(model.Host{Name: "ssh-host", DockerHost: "ssh://host"}); err == nil {
		t.Fatal("expected TCP TLS validation error")
	}
	if _, err := a.create(model.Host{Name: "wrong-port", DockerHost: "tcp://127.0.0.1:2375", TLSCA: "ca", TLSCert: "cert", TLSKey: "key"}); err == nil {
		t.Fatal("expected port validation error")
	}
}
