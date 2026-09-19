package internal

import (
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/gogu-x/ops/model"
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

func TestDemuxDockerLogs(t *testing.T) {
	// Frame: stream type 1 (stdout), 3 zero bytes, 4-byte big-endian size, payload.
	frame := func(streamType byte, payload string) []byte {
		size := len(payload)
		header := []byte{streamType, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
		return append(header, []byte(payload)...)
	}
	raw := append(frame(1, "hello "), frame(2, "world\n")...)
	got := demuxDockerLogs(raw)
	want := "hello world\n"
	if got != want {
		t.Fatalf("demuxDockerLogs() = %q, want %q", got, want)
	}

	// Plain (unframed) text should be returned as-is.
	plain := []byte("plain text without docker framing")
	if got := demuxDockerLogs(plain); got != string(plain) {
		t.Fatalf("demuxDockerLogs(plain) = %q, want %q", got, string(plain))
	}
}

func TestBuildPortBindings(t *testing.T) {
	exposed, bindings, err := buildPortBindings(nil)
	if err != nil || exposed != nil || bindings != nil {
		t.Fatalf("expected nil/nil for no mappings, got %v %v %v", exposed, bindings, err)
	}

	exposed, bindings, err = buildPortBindings([]model.PortMapping{
		{HostPort: "9001", ContainerPort: "9001", Protocol: "tcp"},
		{HostIP: "0.0.0.0", HostPort: "9002", ContainerPort: "9002", Protocol: "udp"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exposed) != 2 {
		t.Fatalf("unexpected exposed ports: %#v", exposed)
	}
	tcpPort, _ := nat.NewPort("tcp", "9001")
	udpPort, _ := nat.NewPort("udp", "9002")
	if _, ok := exposed[tcpPort]; !ok {
		t.Fatalf("expected exposed tcp port, got %#v", exposed)
	}
	if _, ok := exposed[udpPort]; !ok {
		t.Fatalf("expected exposed udp port, got %#v", exposed)
	}
	if len(bindings[tcpPort]) != 1 || bindings[tcpPort][0].HostPort != "9001" {
		t.Fatalf("unexpected tcp binding: %#v", bindings[tcpPort])
	}
	if len(bindings[udpPort]) != 1 || bindings[udpPort][0].HostIP != "0.0.0.0" || bindings[udpPort][0].HostPort != "9002" {
		t.Fatalf("unexpected udp binding: %#v", bindings[udpPort])
	}

	if _, _, err := buildPortBindings([]model.PortMapping{{HostPort: "9001", ContainerPort: "not-a-port"}}); err == nil {
		t.Fatal("expected error for invalid container port")
	}
}
