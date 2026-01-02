package realtime

import (
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

// NewRedis creates a new Redis client
func NewRedis() *redis.Client {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := 0

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	log.Printf("Redis client created (addr: %s)\n", redisAddr)
	return rdb
}
