package entity

import "time"

type Game struct {
	Serial     string    `json:"serial"`
	RedumpHash string    `json:"redump_hash"`
	Title      string    `json:"title"`
	SizeBytes  int64     `json:"size_bytes"`
	FilePath   string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type HealthCheck struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
