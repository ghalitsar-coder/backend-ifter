package discipline

import (
	"errors"
	"time"
)

var (
	ErrNotFound             = errors.New("data not found")
	ErrNotStaff             = errors.New("token subject is not an active staff account")
	ErrForbidden            = errors.New("forbidden")
	ErrInvalidViolationType = errors.New("invalid violation type")
	ErrInvalidStudent       = errors.New("invalid student id")
	ErrInvalidClass         = errors.New("invalid class id")
	ErrInvalidDate          = errors.New("invalid date, expected YYYY-MM-DD")
	ErrInvalidPoints        = errors.New("points must be non-negative")
)

const dateLayout = "2006-01-02"

// Date is a calendar date formatted as "YYYY-MM-DD".
type Date time.Time

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format(dateLayout) + `"`), nil
}

func (d Date) Time() time.Time { return time.Time(d) }

func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return Date{}, ErrInvalidDate
	}
	return Date(parsed), nil
}

// Actor represents the authenticated caller from JWT.
type Actor struct {
	UserID   int64
	SchoolID int64
	Roles    []string
}

func (a Actor) IsAdminOrKesiswaan() bool {
	for _, r := range a.Roles {
		if r == "admin" || r == "kesiswaan" {
			return true
		}
	}
	return false
}

type ViolationType struct {
	ID          int64     `json:"id"`
	SchoolID    int64     `json:"school_id"`
	Kode        string    `json:"kode"`
	Nama        string    `json:"nama"`
	Kategori    string    `json:"kategori"`
	PoinDefault int32     `json:"poin_default"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type DisciplineInspection struct {
	ID              int64     `json:"id"`
	SchoolID        int64     `json:"school_id"`
	ClassID         int64     `json:"class_id"`
	InspectorID     int64     `json:"inspector_id"`
	Tanggal         Date      `json:"tanggal"`
	JumlahDiperiksa int32     `json:"jumlah_diperiksa"`
	Catatan         *string   `json:"catatan"`
	CreatedAt       time.Time `json:"created_at"`
	NamaKelas       string    `json:"nama_kelas"`
	InspectorName   string    `json:"inspector_name"`
}

type StudentViolation struct {
	ID                int64     `json:"id"`
	SchoolID          int64     `json:"school_id"`
	InspectionID      *int64    `json:"inspection_id"`
	StudentID         int64     `json:"student_id"`
	ViolationTypeID   int64     `json:"violation_type_id"`
	Poin              int32     `json:"poin"`
	Keterangan        *string   `json:"keterangan"`
	TindakLanjut      *string   `json:"tindak_lanjut"`
	Tanggal           Date      `json:"tanggal"`
	CreatedAt         time.Time `json:"created_at"`
	NIS               *string   `json:"nis"`
	StudentName       string    `json:"student_name"`
	NamaKelas         *string   `json:"nama_kelas"`
	ViolationCode     string    `json:"violation_code"`
	ViolationName     string    `json:"violation_name"`
	ViolationCategory string    `json:"violation_category"`
}

type StudentSummary struct {
	StudentID      int64  `json:"student_id"`
	TotalPoints    int64  `json:"total_points"`
	TotalIncidents int64  `json:"total_incidents"`
	StatusSP       string `json:"status_sp"`
}

type ClassStudent struct {
	ID           int64   `json:"id"`
	NIS          *string `json:"nis"`
	Nama         string  `json:"nama"`
	JenisKelamin string  `json:"jenis_kelamin"`
	NamaKelas    string  `json:"nama_kelas"`
}

type RecordViolationInput struct {
	InspectionID    *int64  `json:"inspection_id"`
	StudentID       int64   `json:"student_id"`
	ViolationTypeID int64   `json:"violation_type_id"`
	Poin            *int32  `json:"poin"`
	Keterangan      *string `json:"keterangan"`
	TindakLanjut    *string `json:"tindak_lanjut"`
	Tanggal         string  `json:"tanggal"`
}

type CreateInspectionViolationItem struct {
	StudentID       int64   `json:"student_id"`
	ViolationTypeID int64   `json:"violation_type_id"`
	Poin            *int32  `json:"poin"`
	Keterangan      *string `json:"keterangan"`
}

type CreateInspectionInput struct {
	ClassID         int64                           `json:"class_id"`
	Tanggal         string                          `json:"tanggal"`
	JumlahDiperiksa int32                           `json:"jumlah_diperiksa"`
	Catatan         *string                         `json:"catatan"`
	Violations      []CreateInspectionViolationItem `json:"violations"`
}

type ListViolationsFilter struct {
	StudentID *int64
	ClassID   *int64
	Kategori  *string
}

type ListInspectionsFilter struct {
	ClassID *int64
	Tanggal *Date
}

// CalculateSPStatus determines sanction advisory tier based on accumulated points.
func CalculateSPStatus(totalPoints int64) string {
	switch {
	case totalPoints >= 100:
		return "SP3 (Dikeluarkan / Pindah Sekolah)"
	case totalPoints >= 75:
		return "SP2 (Skorsing 3 Hari)"
	case totalPoints >= 50:
		return "SP1 (Surat Peringatan 1)"
	case totalPoints >= 30:
		return "Panggilan Orang Tua"
	case totalPoints >= 15:
		return "Peringatan Lisan / Pembinaan Wali Kelas"
	default:
		return "Aman"
	}
}
