package conf

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
)

type Config struct {
	Addr           string
	MongoURI       string
	MongoDatabase  string
	MongoUsername  string
	MongoPassword  string
	JWTSecret      string
	AccessTTLMin   int
	RefreshTTLDays int
	AdminPassword  string
}

func LoadConfig() Config {
	return Config{
		Addr:           envString("OPS_ADDR", ":8090"),
		MongoURI:       os.Getenv("OPS_MONGO_URI"),
		MongoDatabase:  envString("OPS_MONGO_DATABASE", "ops_platform"),
		MongoUsername:  os.Getenv("OPS_MONGO_USERNAME"),
		MongoPassword:  os.Getenv("OPS_MONGO_PASSWORD"),
		JWTSecret:      envString("OPS_JWT_SECRET", RandomSecret()),
		AccessTTLMin:   envInt("OPS_ACCESS_TTL_MIN", 15),
		RefreshTTLDays: envInt("OPS_REFRESH_TTL_DAYS", 7),
		AdminPassword:  os.Getenv("OPS_ADMIN_PASSWORD"),
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func RandomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "development-secret-change-me"
	}
	return hex.EncodeToString(buf)
}
