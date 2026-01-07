package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppPort         string
	DBDSN           string
	JWTSecret       string
	JWTExpiresMin   int
	GoogleClientID  string
	GoogleSecret    string
	GoogleRedirect  string
	FrontendBaseURL string
}

func Load() Config {
	expires, _ := strconv.Atoi(get("JWT_EXPIRES_MIN", "10080"))
	return Config{
		AppPort:         get("APP_PORT", "8080"),
		DBDSN:           must("DB_DSN"),
		JWTSecret:       must("JWT_SECRET"),
		JWTExpiresMin:   expires,
		GoogleClientID:  get("GOOGLE_CLIENT_ID", ""),
		GoogleSecret:    get("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirect:  get("GOOGLE_REDIRECT_URL", ""),
		FrontendBaseURL: get("FRONTEND_BASE_URL", "http://localhost:3000"),
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
