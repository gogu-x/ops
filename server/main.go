package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gogu-x/ops/conf"
	"github.com/gogu-x/ops/ops"
	"github.com/gogu-x/tree"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "ops",
		Usage: "ops platform",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Usage: "HTTP listen address", Sources: cli.EnvVars("OPS_ADDR")},
			&cli.StringFlag{Name: "mongo-uri", Usage: "MongoDB URI", Sources: cli.EnvVars("OPS_MONGO_URI")},
			&cli.StringFlag{Name: "jwt-secret", Usage: "JWT signing secret", Sources: cli.EnvVars("OPS_JWT_SECRET")},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			cfg := conf.LoadConfig()
			if c.IsSet("addr") {
				cfg.Addr = c.String("addr")
			}
			if c.IsSet("mongo-uri") {
				cfg.MongoURI = c.String("mongo-uri")
			}
			if c.IsSet("jwt-secret") {
				cfg.JWTSecret = c.String("jwt-secret")
			}
			system, err := ops.NewSystem(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() {
				closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := system.Close(closeCtx); err != nil {
					log.Printf("close application: %v", err)
				}
			}()
			tree.Spawn(system.Actors...)
			tree.Default().Start()
			return nil
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
