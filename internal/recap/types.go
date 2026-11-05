package recap

import (
	"errors"
	"time"
)

var (
	ErrForbidden     = errors.New("school scope is required")
	ErrInvalidPeriod = errors.New("invalid recap period")
	ErrInvalidID     = errors.New("invalid recap id")
	ErrNotFound      = errors.New("student attendance recap not found")
	ErrLocked        = errors.New("student attendance recap is already locked")
)

type StudentRecap struct {
	ID             int64     `json:"id"`
	StudentID      int64     `json:"student_id"`
	StudentName    string    `json:"student_name"`
	ClassID        int64     `json:"class_id"`
	ClassName      string    `json:"class_name"`
	Month          int16     `json:"bulan"`
	Year           int16     `json:"tahun"`
	TotalHadir     int32     `json:"total_hadir"`
	TotalSakit     int32     `json:"total_sakit"`
	TotalIzin      int32     `json:"total_izin"`
	TotalAlpa      int32     `json:"total_alpa"`
	AttendanceRate float64   `json:"persentase_hadir"`
	Status         string    `json:"status_kunci"`
	VerifiedBy     *int64    `json:"verified_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type StaffRecap struct {
	ID             int64     `json:"id"`
	StaffID        int64     `json:"staff_id"`
	StaffName      string    `json:"nama_pegawai"`
	NIP            *string   `json:"nip"`
	Month          int16     `json:"bulan"`
	Year           int16     `json:"tahun"`
	TotalWorkdays  int32     `json:"total_hari_kerja"`
	TotalHadir     int32     `json:"total_hadir"`
	TotalTerlambat int32     `json:"total_terlambat"`
	TotalIzin      int32     `json:"total_izin"`
	TotalSakit     int32     `json:"total_sakit"`
	TotalAlpa      int32     `json:"total_alpa"`
	AttendanceRate float64   `json:"persentase_hadir"`
	Status         string    `json:"status_kunci"`
	VerifiedBy     *int64    `json:"verified_by"`
	CreatedAt      time.Time `json:"created_at"`
}

type Verification struct {
	ID         int64  `json:"id"`
	Status     string `json:"status_kunci"`
	VerifiedBy *int64 `json:"verified_by"`
}
