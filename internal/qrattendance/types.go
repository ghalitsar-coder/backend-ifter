package qrattendance

import (
	"context"
	"errors"
	"time"
)

var (
	ErrForbidden       = errors.New("not allowed to manage QR attendance")
	ErrInvalidSession  = errors.New("invalid QR session")
	ErrInvalidDate     = errors.New("invalid session date")
	ErrInvalidTime     = errors.New("invalid session time")
	ErrInvalidRotation = errors.New("invalid QR rotation interval")
	ErrNotFound        = errors.New("QR session or active staff not found")
	ErrSessionExists   = errors.New("QR session already exists for date and type")
	ErrSessionClosed   = errors.New("QR session is closed")
	ErrOutsideWindow   = errors.New("QR session is outside its attendance window")
	ErrInvalidCode     = errors.New("invalid QR code")
	ErrExpiredCode     = errors.New("expired QR code")
	ErrAlreadyScanned  = errors.New("attendance has already been recorded")
	ErrCheckInRequired = errors.New("check-in is required before check-out")
)

type CreateSessionInput struct {
	Date            string  `json:"tanggal"`
	Type            string  `json:"tipe_sesi"`
	OpenAt          string  `json:"jam_buka"`
	CloseAt         string  `json:"jam_tutup"`
	LateAfter       *string `json:"batas_terlambat,omitempty"`
	RotationSeconds *int    `json:"rotasi_detik,omitempty"`
}

type Session struct {
	ID              int64     `json:"id"`
	Date            string    `json:"tanggal"`
	Type            string    `json:"tipe_sesi"`
	OpenAt          string    `json:"jam_buka"`
	CloseAt         string    `json:"jam_tutup"`
	LateAfter       *string   `json:"batas_terlambat,omitempty"`
	RotationSeconds int32     `json:"rotasi_detik"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type SessionRecord struct {
	Session
	SchoolID    int64
	CreatedBy   int64
	TokenSecret string `json:"-"`
}

type SessionCreator struct {
	UserID   int64
	SchoolID int64
}

type QRCode struct {
	SessionID  int64     `json:"session_id"`
	Payload    string    `json:"payload"`
	ImagePNG   string    `json:"image_png"`
	ValidUntil time.Time `json:"valid_until"`
}

type ScanInput struct {
	UserID   int64
	SchoolID int64
	Payload  string
}

type ScanResult struct {
	StaffID   int64     `json:"staff_id"`
	Type      string    `json:"tipe_sesi"`
	Date      string    `json:"tanggal"`
	Status    string    `json:"status"`
	ScannedAt time.Time `json:"waktu_scan"`
}

type Repository interface {
	FindOpenSession(ctx context.Context, schoolID int64, date time.Time, clock string) (SessionRecord, error)
	CreateSession(ctx context.Context, schoolID, creatorID int64, secret string, input CreateSessionInput, date time.Time, openAt, closeAt string, lateAfter *string) (SessionRecord, error)
	GetSession(ctx context.Context, id, schoolID int64) (SessionRecord, error)
	CloseSession(ctx context.Context, id, schoolID int64) error
	FindActiveStaff(ctx context.Context, userID, schoolID int64) (int64, error)
	CheckIn(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time, status string) error
	CheckOut(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time) error
}
