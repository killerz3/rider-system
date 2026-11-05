package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const defaultH3Resolution = 7
const operationTimeout = 3 * time.Second

type config struct {
	RedisAddr, HTTPPort, SQLitePath, SearchMode string
	H3Resolution, GridDistance                  int
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func loadConfig() (config, error) {
	c := config{RedisAddr: env("REDIS_ADDR", "localhost:6379"), HTTPPort: env("HTTP_PORT", "8080"), SQLitePath: env("SQLITE_PATH", "riders.db")}
	var err error
	c.H3Resolution, err = strconv.Atoi(env("H3_RESOLUTION", strconv.Itoa(defaultH3Resolution)))
	if err != nil || c.H3Resolution < 0 || c.H3Resolution > 15 {
		return c, fmt.Errorf("H3_RESOLUTION must be 0..15")
	}
	c.GridDistance, err = strconv.Atoi(env("GRID_DISTANCE", "2"))
	if err != nil || c.GridDistance < 0 || c.GridDistance > 20 {
		return c, fmt.Errorf("GRID_DISTANCE must be 0..20")
	}
	c.SearchMode = env("SEARCH_MODE", "rounds")
	if c.SearchMode != "rounds" && c.SearchMode != "disk" {
		return c, fmt.Errorf("SEARCH_MODE must be rounds or disk")
	}
	return c, nil
}
