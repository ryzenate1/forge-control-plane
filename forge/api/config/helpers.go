package config

import (
	"log"
	"os"
	"strconv"
)

func env(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		log.Printf("WARNING: invalid integer value for %s=%q, using fallback %d", key, val, fallback)
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
		log.Printf("WARNING: invalid boolean value for %s=%q, using fallback %t", key, val, fallback)
	}
	return fallback
}
