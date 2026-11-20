package autoopen

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogu-x/ops/model"
)

func TestScheduledInstanceInheritsGameConnectionFlagsAndMasksPreview(t *testing.T) {
	const password = "example-test-password"
	template := model.ServiceInstance{
		Name: "game-1", ServiceTypeID: "game", HostID: "test", Image: "game:test", Network: "game-net",
		Params: []model.ServiceParam{{Flag: "--node-id", Value: "1"}, {Flag: "--gconf-db", Value: "gs_conf_test"}},
		EnvText: strings.Join([]string{
			"--server-id 1 \\", "--etcd etcd:2379 \\", "--nats-url=nats://token-test@nats:4222 \\",
			"--mongo-url mongodb://db:27018 \\", "--mongo-username sample \\", "--mongo-password " + password + " \\",
			"--gconf-db-uri mongodb://sample:" + password + "@db:27018/?authSource=admin&secret=query-test \\",
			"--platform-grpc-addr platform:7000",
		}, "\n"),
	}
	policy := model.AutoOpenPolicy{AutoOpenSettings: model.AutoOpenSettings{NamePrefix: "game-", FirstServerID: 101}}
	created, err := scheduledInstance(template, policy, 0, 123456789)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range gameConnectionFlags {
		if want := gameFlagValue(template, flag); want == "" || gameFlagValue(created, flag) != want {
			t.Errorf("%s was not inherited", flag)
		}
	}
	if gameFlagValue(created, "--server-id") != "101" || gameFlagValue(created, "--grpc-host") != "game-101" ||
		gameFlagValue(created, "--server-open-ts") != "123456789" {
		t.Fatal("scheduled flags must be generated from the policy")
	}

	preview := templateParameters(template)
	for _, param := range preview {
		if !param.Configured {
			t.Errorf("%s is missing from the preview", param.Flag)
		}
	}
	data, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, secret := range []string{password, "token-test", "query-test", "authSource"} {
		if strings.Contains(serialized, secret) {
			t.Fatal("the preview contains sensitive connection data")
		}
	}
	for _, value := range []string{"etcd:2379", "nats://nats:4222", "mongodb://db:27018", "gs_conf_test", "platform:7000"} {
		if !strings.Contains(serialized, value) {
			t.Errorf("the preview is missing %s", value)
		}
	}
	if templateParameters(model.ServiceInstance{})[0].Configured {
		t.Fatal("an unset --node-id was reported as configured")
	}
}
