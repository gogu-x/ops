package ops

import (
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops/internal"
	"github.com/gogu-x/tree"
)

func NewPos(cfg conf.Config) tree.Actor {
	return internal.NewActor(cfg)
}
