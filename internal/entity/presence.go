package entity

import "time"

type Presence struct {
	UserID    string    `json:"user_id"`
	GameID    string    `json:"game_id"`
	State     string    `json:"state"`
	RoomID    string    `json:"room_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Room struct {
	ID      string   `json:"id"`
	GameID  string   `json:"game_id"`
	VNI     uint32   `json:"vni"`
	Members []string `json:"members"`
}
