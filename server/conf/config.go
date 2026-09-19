package conf

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/urfave/cli/v3"
)

var (
	// host（容器内需设为容器名或IP）
	Addr = "5001"

	// MongoURL MongoDB 连接地址
	MongoURL = "mongodb://43.160.212.55:27017"

	// MongoUsername MongoDB 认证用户名
	MongoUsername = ""

	// MongoPassword MongoDB 认证密码
	MongoPassword = ""

	// NatsURL NATS 连接地址
	NatsURL = "nats://43.160.212.55:4222"

	// JWTSecret JWT 签名密钥
	JWTSecret = "jwt-secret"

	AdminPassword = "admin-password"

	AccessTTLMin = 15

	RefreshTTLDays = 7
)

// ConnectionFlags returns flags shared by all service entrypoints. Values passed
// explicitly on the command line override values from --config.
func ConnectionFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "addr", Usage: "addr"},
		&cli.StringFlag{Name: "etcd", Usage: "etcd, comma-separated"},
		&cli.StringFlag{Name: "grpc-host", Usage: "game gRPC host"},
		&cli.StringFlag{Name: "mongo-url", Usage: "MongoDB connection URI"},
		&cli.StringFlag{Name: "mongo-username", Usage: "MongoDB authentication username"},
		&cli.StringFlag{Name: "mongo-password", Usage: "MongoDB authentication password"},
		&cli.StringFlag{Name: "nats-url", Usage: "NATS connection URI"},
		&cli.StringFlag{Name: "jwt-secret", Usage: "JWT signing secret"},
		&cli.StringFlag{Name: "admin-password", Usage: "admin-password"},
	}
}

// LoadAndApply loads --config first, then applies only explicitly supplied
// command-line connection settings, giving the command line precedence.
func LoadAndApply(c *cli.Command) error {

	if c.IsSet("addr") {
		Addr = c.String("addr")
	}
	if c.IsSet("mongo-url") {
		MongoURL = c.String("mongo-url")
	}
	if c.IsSet("mongo-username") {
		MongoUsername = c.String("mongo-username")
	}
	if c.IsSet("mongo-password") {
		MongoPassword = c.String("mongo-password")
	}
	if c.IsSet("nats-url") {
		NatsURL = c.String("nats-url")
	}
	if c.IsSet("jwt-secret") {
		JWTSecret = c.String("jwt-secret")
	}
	if c.IsSet("admin-password") {
		AdminPassword = c.String("admin-password")
	}
	return nil
}

func splitEndpoints(value string) []string {
	endpoints := strings.Split(value, ",")
	for i := range endpoints {
		endpoints[i] = strings.TrimSpace(endpoints[i])
	}
	return endpoints
}

func RandomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "development-secret-change-me"
	}
	return hex.EncodeToString(buf)
}
