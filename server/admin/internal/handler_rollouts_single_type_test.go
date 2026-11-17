package internal

import (
	"github.com/gogu-x/ops/model"
	"testing"
)

func TestRolloutHasSingleServiceType(t *testing.T) {
	tests := []struct {
		name      string
		instances []model.ServiceInstance
		want      bool
	}{
		{"empty", nil, false},
		{"same type", []model.ServiceInstance{{ServiceTypeID: "game"}, {ServiceTypeID: "game"}}, true},
		{"different types", []model.ServiceInstance{{ServiceTypeID: "game"}, {ServiceTypeID: "gate"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rolloutHasSingleServiceType(tt.instances); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
