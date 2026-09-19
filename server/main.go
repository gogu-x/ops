package main

import (
	"context"
	"log"
	"os"

	"github.com/gogu-x/ops/admin"
	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/host"
	"github.com/gogu-x/ops/instance"
	"github.com/gogu-x/tree"
	"github.com/gogu-x/tree/db/mongorpc"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "ops",
		Usage: "ops platform",
		Flags: conf.ConnectionFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			if err := conf.LoadAndApply(c); err != nil {
				return err
			}

			mongodbData := mongorpc.Connect(conf.MongoURL, conf.MongoUsername, conf.MongoPassword, "ops_platform")
			if mongodbData == nil {
				return nil
			}

			actors := make([]tree.Actor, 0, 4)
			actors = append(actors,
				admin.NewAdminService(mongodbData),
				host.NewHostService(mongodbData),
				instance.NewInstanceService(mongodbData),
			)
			tree.Spawn(actors...)
			tree.Default().Start()
			return nil
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
