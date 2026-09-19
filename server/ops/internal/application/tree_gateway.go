package application

import (
	"errors"
	"time"

	"github.com/gogu-x/tree"
)

type TreeGateway struct {
	timeout time.Duration
}

func NewTreeGateway(timeout time.Duration) *TreeGateway {
	return &TreeGateway{timeout: timeout}
}

func (g *TreeGateway) Request(actor string, message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup(actor)
	if !ok {
		return nil, errors.New(unavailableMessage(actor))
	}
	return tree.Request(pid, message).AwaitTimeout(g.timeout)
}

func unavailableMessage(actor string) string {
	switch actor {
	case "ops-host":
		return "host actor is unavailable"
	case "ops-instance":
		return "service instance actor is unavailable"
	default:
		return actor + " actor is unavailable"
	}
}
