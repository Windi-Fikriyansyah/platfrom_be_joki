package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppPort       string
	DBDSN         string
	JWTSecret     string
	JWTExpiresMin int
}

func Load() Config {
	expires, _ := strconv.Atoi(get("JWT_EXPIRES_MIN", "10080"))
	return Config{
		AppPort:       get("APP_PORT", "8080"),
		DBDSN:         must("DB_DSN"),
		JWTSecret:     must("JWT_SECRET"),
		JWTExpiresMin: expires,
	}
}

func get(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
func must(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic("missing env: " + k)
	}
	return v
}
