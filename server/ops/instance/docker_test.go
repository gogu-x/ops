package instance

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/gogu-x/ops/ops/host"
	"github.com/gogu-x/ops/ops/internal/model"
)

func TestMapContainerStatus(t *testing.T) {
	cases := map[string]string{
		"running":    model.StatusRunning,
		"restarting": model.StatusRestarting,
		"exited":     model.StatusStopped,
		"created":    model.StatusStopped,
		"paused":     model.StatusStopped,
		"dead":       model.StatusStopped,
		"removing":   model.StatusStopped,
		"":           model.StatusNotDeployed,
		"something":  model.StatusUnknown,
	}
	for state, want := range cases {
		if got := mapContainerStatus(state); got != want {
			t.Errorf("mapContainerStatus(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestBuildContainerSpec(t *testing.T) {
	item := model.ServiceInstance{
		ID:          "abc-123",
		Name:        "game-1",
		Image:       "gogs-game:v1",
		Network:     "ops-net",
		PortMapping: "9003:9004",
		Params: []model.ServiceParam{
			{Flag: "--server-id", Value: "1001"},
			{Flag: "--zone", Value: "cn"},
		},
		EnvText: "MONGO_URL=mongodb://172.17.0.1:27018\n--etcd etcd:2379 \\\n-p 9001:9001\n--publish 9002:9002/udp\nAPP_ENV=prod\n\n",
	}
	spec := buildContainerSpec(item)

	if spec.Name != "game-1" {
		t.Fatalf("unexpected container name: %s, want instance name", spec.Name)
	}
	if spec.Image != "gogs-game:v1" {
		t.Fatalf("unexpected image: %s", spec.Image)
	}
	if spec.NetworkMode != "ops-net" {
		t.Fatalf("unexpected network mode: %s, want dedicated Network field to be used", spec.NetworkMode)
	}
	wantCmd := []string{"--server-id", "1001", "--zone", "cn", "--etcd", "etcd:2379"}
	if len(spec.Cmd) != len(wantCmd) {
		t.Fatalf("unexpected cmd: %#v", spec.Cmd)
	}
	for i, v := range wantCmd {
		if spec.Cmd[i] != v {
			t.Fatalf("cmd[%d] = %q, want %q (full: %#v)", i, spec.Cmd[i], v, spec.Cmd)
		}
	}
	wantEnv := []string{"MONGO_URL=mongodb://172.17.0.1:27018", "APP_ENV=prod"}
	if len(spec.Env) != len(wantEnv) {
		t.Fatalf("unexpected env: %#v", spec.Env)
	}
	for i, v := range wantEnv {
		if spec.Env[i] != v {
			t.Fatalf("env[%d] = %q, want %q (full: %#v)", i, spec.Env[i], v, spec.Env)
		}
	}
	// 2 from EnvText ("-p"/"--publish" lines) + 1 from the dedicated
	// PortMapping field, appended last.
	if len(spec.Ports) != 3 {
		t.Fatalf("unexpected ports: %#v", spec.Ports)
	}
	if spec.Ports[0].HostPort != "9001" || spec.Ports[0].ContainerPort != "9001" || spec.Ports[0].Protocol != "tcp" {
		t.Fatalf("unexpected port[0]: %#v", spec.Ports[0])
	}
	if spec.Ports[1].HostPort != "9002" || spec.Ports[1].ContainerPort != "9002" || spec.Ports[1].Protocol != "udp" {
		t.Fatalf("unexpected port[1]: %#v", spec.Ports[1])
	}
	if spec.Ports[2].HostPort != "9003" || spec.Ports[2].ContainerPort != "9004" || spec.Ports[2].Protocol != "tcp" {
		t.Fatalf("unexpected port[2] (dedicated field): %#v", spec.Ports[2])
	}
	if spec.Labels[host.OpsInstanceLabel] != "abc-123" {
		t.Fatalf("expected instance label, got %#v", spec.Labels)
	}
}

func TestParsePortMappingField(t *testing.T) {
	cases := []struct {
		raw       string
		wantOK    bool
		wantHost  string
		wantCont  string
		wantProto string
	}{
		{"9001:9001", true, "9001", "9001", "tcp"},
		{"9001:9002", true, "9001", "9002", "tcp"},
		{"9001", true, "9001", "9001", "tcp"},
		{"9001:9001/udp", true, "9001", "9001", "udp"},
		{"", false, "", "", ""},
		{"  ", false, "", "", ""},
		{"9001:9002:9003", false, "", "", ""},
	}
	for _, tc := range cases {
		mapping, ok := parsePortMappingField(tc.raw)
		if ok != tc.wantOK {
			t.Errorf("parsePortMappingField(%q) ok = %v, want %v", tc.raw, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if mapping.HostPort != tc.wantHost || mapping.ContainerPort != tc.wantCont || mapping.Protocol != tc.wantProto {
			t.Errorf("parsePortMappingField(%q) = %#v, want {%q %q %q}", tc.raw, mapping, tc.wantHost, tc.wantCont, tc.wantProto)
		}
	}
}

func TestParsePortMappingLine(t *testing.T) {
	cases := []struct {
		line       string
		wantOK     bool
		wantHostIP string
		wantHost   string
		wantCont   string
		wantProto  string
	}{
		{"-p 9001:9001", true, "", "9001", "9001", "tcp"},
		{"-p 9001:9001/udp", true, "", "9001", "9001", "udp"},
		{"--publish 0.0.0.0:9001:9002", true, "0.0.0.0", "9001", "9002", "tcp"},
		{"MONGO_URL=mongodb://172.17.0.1:27018", false, "", "", "", ""},
		{"--etcd etcd:2379", false, "", "", "", ""},
		{"-p", false, "", "", "", ""},
	}
	for _, tc := range cases {
		mapping, ok := parsePortMappingLine(tc.line)
		if ok != tc.wantOK {
			t.Errorf("parsePortMappingLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if mapping.HostIP != tc.wantHostIP || mapping.HostPort != tc.wantHost || mapping.ContainerPort != tc.wantCont || mapping.Protocol != tc.wantProto {
			t.Errorf("parsePortMappingLine(%q) = %#v, want {%q %q %q %q}", tc.line, mapping, tc.wantHostIP, tc.wantHost, tc.wantCont, tc.wantProto)
		}
	}
}

func TestFindContainerForInstance(t *testing.T) {
	containers := []types.Container{
		{ID: "c1", Labels: map[string]string{host.OpsInstanceLabel: "inst-1"}},
		{ID: "c2", Labels: map[string]string{host.OpsInstanceLabel: "inst-2"}},
	}
	c, found := findContainerForInstance(containers, "inst-2")
	if !found || c.ID != "c2" {
		t.Fatalf("expected to find c2, got %+v found=%v", c, found)
	}
	_, found = findContainerForInstance(containers, "missing")
	if found {
		t.Fatal("expected not found for missing instance id")
	}
}

func TestApplyContainerSummary(t *testing.T) {
	item := model.ServiceInstance{ID: "inst-1"}
	c := types.Container{ID: "container-id", State: "running", Status: "Up 5 minutes", Image: "gogs-game:v1"}
	applyContainerSummary(&item, c)
	if item.ContainerID != "container-id" || item.Status != model.StatusRunning || item.StatusText != "Up 5 minutes" || item.ContainerImage != "gogs-game:v1" {
		t.Fatalf("unexpected enriched instance: %+v", item)
	}
}

func TestBuildContainerDetail(t *testing.T) {
	c := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:           "container-id",
			Image:        "gogs-game:v1",
			Name:         "/ops-abc-123",
			RestartCount: 2,
			Created:      "2026-01-01T00:00:00Z",
			HostConfig: &container.HostConfig{
				NetworkMode:   "bridge",
				RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
			},
			State: &types.ContainerState{
				Status:     "running",
				Running:    true,
				StartedAt:  "2026-01-01T00:00:01Z",
				FinishedAt: "",
				ExitCode:   0,
			},
		},
		Config: &container.Config{
			Cmd:    []string{"--server-id", "1001"},
			Env:    []string{"MONGO_URL=mongodb://172.17.0.1:27018"},
			Labels: map[string]string{host.OpsInstanceLabel: "abc-123"},
		},
		NetworkSettings: &types.NetworkSettings{
			NetworkSettingsBase: types.NetworkSettingsBase{
				Ports: nat.PortMap{
					"7000/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "7000"}},
				},
			},
			DefaultNetworkSettings: types.DefaultNetworkSettings{IPAddress: "172.17.0.5"},
		},
		Mounts: []types.MountPoint{
			{Source: "/data/game-1", Destination: "/app/data", Mode: "rw"},
		},
	}

	detail := buildContainerDetail(c)

	if detail.ContainerID != "container-id" || detail.Name != "ops-abc-123" || detail.Image != "gogs-game:v1" {
		t.Fatalf("unexpected base fields: %+v", detail)
	}
	if detail.Status != model.StatusRunning || !detail.Running || detail.RestartCount != 2 {
		t.Fatalf("unexpected state fields: %+v", detail)
	}
	if detail.NetworkMode != "bridge" || detail.RestartPolicy != "unless-stopped" {
		t.Fatalf("unexpected host config fields: %+v", detail)
	}
	if detail.IPAddress != "172.17.0.5" {
		t.Fatalf("unexpected ip address: %+v", detail)
	}
	if len(detail.Ports) != 1 || detail.Ports[0] != "0.0.0.0:7000 -> 7000/tcp" {
		t.Fatalf("unexpected ports: %+v", detail.Ports)
	}
	if len(detail.Mounts) != 1 || detail.Mounts[0] != "/data/game-1 -> /app/data (rw)" {
		t.Fatalf("unexpected mounts: %+v", detail.Mounts)
	}
	if len(detail.Env) != 1 || detail.Env[0] != "MONGO_URL=mongodb://172.17.0.1:27018" {
		t.Fatalf("unexpected env: %+v", detail.Env)
	}
	if detail.Labels[host.OpsInstanceLabel] != "abc-123" {
		t.Fatalf("unexpected labels: %+v", detail.Labels)
	}
}

func TestBuildContainerDetailNormalizesNilSlices(t *testing.T) {
	// A container created with no Cmd, no Env, no exposed/published ports and
	// no mounts should still return non-nil empty slices so the JSON
	// response encodes "[]" rather than "null" (frontend calls .length on
	// these fields without null-checking).
	c := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    "container-id",
			Image: "gogs-game:v1",
			Name:  "/game-1",
			State: &types.ContainerState{Status: "running", Running: true},
		},
		Config:          &container.Config{},
		NetworkSettings: &types.NetworkSettings{},
	}

	detail := buildContainerDetail(c)

	if detail.Cmd == nil || len(detail.Cmd) != 0 {
		t.Fatalf("expected non-nil empty Cmd, got %#v", detail.Cmd)
	}
	if detail.Env == nil || len(detail.Env) != 0 {
		t.Fatalf("expected non-nil empty Env, got %#v", detail.Env)
	}
	if detail.Ports == nil || len(detail.Ports) != 0 {
		t.Fatalf("expected non-nil empty Ports, got %#v", detail.Ports)
	}
	if detail.Mounts == nil || len(detail.Mounts) != 0 {
		t.Fatalf("expected non-nil empty Mounts, got %#v", detail.Mounts)
	}
	if detail.Labels == nil {
		t.Fatalf("expected non-nil Labels map, got %#v", detail.Labels)
	}
}
