package db

import "time"

type Email struct {
	ID                 string `gorm:"primaryKey"`
	CreatedAt          time.Time
	ExpiresAt          time.Time
	Timestamp          string
	ExpiredMessageSent bool `gorm:"default:false"`
}
