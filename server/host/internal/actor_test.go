package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/gogu-x/ops/model"
)

func tlsHost(name string) model.Host {
	return model.Host{Name: name, DockerHost: "tcp://127.0.0.1:2376", TLSCA: "ca.pem", TLSCert: "client-cert.pem", TLSKey: "client-key.pem"}
}

func TestHostConfigurationLifecycle(t *testing.T) {
	a := NewDockerService()
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
	a := NewDockerService()
	if _, err := a.create(model.Host{Name: "ssh-host", DockerHost: "ssh://host"}); err == nil {
		t.Fatal("expected TCP TLS validation error")
	}
	if _, err := a.create(model.Host{Name: "wrong-port", DockerHost: "tcp://127.0.0.1:2375", TLSCA: "ca", TLSCert: "cert", TLSKey: "key"}); err == nil {
		t.Fatal("expected port validation error")
	}
}

func TestProjectHostBindingsAllowMultipleProjects(t *testing.T) {
	a := NewDockerService()
	host1, err := a.create(tlsHost("docker-1"))
	if err != nil {
		t.Fatal(err)
	}
	host2, err := a.create(tlsHost("docker-2"))
	if err != nil {
		t.Fatal(err)
	}

	if err := a.setProjectHosts("project-1", []string{host1.ID, host2.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.setProjectHosts("project-2", []string{host1.ID}); err != nil {
		t.Fatalf("expected a host to be shared across projects, got %v", err)
	}

	bound, err := a.repo.Get(context.Background(), host1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.HasProject("project-1") || !bound.HasProject("project-2") {
		t.Fatalf("host project bindings = %#v, want both projects", bound.ProjectIDs)
	}
	if err := a.setProjectHosts("project-1", []string{host2.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.setProjectHosts("project-1", nil); err != nil {
		t.Fatal(err)
	}
	bound, err = a.repo.Get(context.Background(), host1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.HasProject("project-1") || !bound.HasProject("project-2") {
		t.Fatalf("updating one project's bindings must preserve other project memberships: %#v", bound.ProjectIDs)
	}
}
