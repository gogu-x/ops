package ops

import (
	"context"
	"reflect"
	"testing"

	"github.com/gogu-x/ops/conf"
)

func TestSystemRegistersOnlyHTTPAndRuntimeActors(t *testing.T) {
	system, err := NewSystem(context.Background(), conf.Config{
		JWTSecret:      "test-secret",
		AccessTTLMin:   15,
		RefreshTTLDays: 7,
		AdminPassword:  "admin-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = system.Close(context.Background()) })

	names := make([]string, 0, len(system.Actors))
	for _, actor := range system.Actors {
		names = append(names, actor.Name())
	}
	want := []string{"ops-http", "ops-host", "ops-instance"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("actor names = %#v, want %#v", names, want)
	}
}
