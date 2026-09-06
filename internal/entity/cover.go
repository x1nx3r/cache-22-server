package entity

import "time"

type Cover struct {
	Serial     string    `json:"serial"`
	Provider   string    `json:"provider"`
	ProviderID int64     `json:"provider_id"`
	ImagePath  string    `json:"-"`
	FetchedAt  time.Time `json:"fetched_at"`
}
