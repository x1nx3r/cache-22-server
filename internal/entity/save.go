package entity

import "time"

type Save struct {
	UserID    int64     `json:"-"`
	Serial    string    `json:"serial"`
	Slot      int       `json:"slot"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Public    bool      `json:"public"`
	UpdatedAt time.Time `json:"updated_at"`
}
