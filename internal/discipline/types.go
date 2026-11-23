package discipline

import (
	"errors"
	"time"
)

var (
	ErrNotFound             = errors.New("data not found")
	ErrConflict             = errors.New("resource already exists or is referenced")
	ErrNotStaff             = errors.New("token subject is not an active staff account")
	ErrForbidden            = errors.New("forbidden")
	ErrInvalidViolationType = errors.New("invalid violation type")
	ErrInvalidCategory      = errors.New("invalid category, expected atribut, kerapihan, perilaku, or lainnya")
	ErrInvalidStatus        = errors.New("invalid status")
	ErrInvalidStudent       = errors.New("invalid student id")
	ErrInvalidClass         = errors.New("invalid class id")
	ErrInvalidDate          = errors.New("invalid date, expected YYYY-MM-DD")
	ErrInvalidDateTime      = errors.New("invalid datetime, expected RFC3339 or YYYY-MM-DD HH:MM")
	ErrInvalidPoints        = errors.New("points must be non-negative")
	ErrInvalidPayload       = errors.New("required fields must not be empty")
)

const (
	dateLayout         = "2006-01-02"
	dateTimeMinLayout  = "2006-01-02T15:04"
	dateTimeSpcLayout  = "2006-01-02 15:04"
	dateTimeSecLayout  = "2006-01-02T15:04:05"
)

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

func ParseDateTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	for _, layout := range []string{dateTimeMinLayout, dateTimeSpcLayout, dateTimeSecLayout} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, ErrInvalidDateTime
}

// Actor represents the authenticated caller from JWT.
type Actor struct {
	UserID   int64
	SchoolID int64
	Roles    []string
}

// IsKesiswaan reports whether the authenticated caller holds the kesiswaan role.
func (a Actor) IsKesiswaan() bool {
	for _, r := range a.Roles {
		if r == "kesiswaan" {
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

type UpsertViolationTypeInput struct {
	Kode        string `json:"kode"`
	Nama        string `json:"nama"`
	Kategori    string `json:"kategori"`
	PoinDefault int32  `json:"poin_default"`
	IsActive    *bool  `json:"is_active"`
}

type DisciplineInspection struct {
	ID              int64              `json:"id"`
	SchoolID        int64              `json:"school_id"`
	ClassID         int64              `json:"class_id"`
	InspectorID     int64              `json:"inspector_id"`
	Tanggal         Date               `json:"tanggal"`
	JumlahDiperiksa int32              `json:"jumlah_diperiksa"`
	Catatan         *string            `json:"catatan"`
	CreatedAt       time.Time          `json:"created_at"`
	NamaKelas       string             `json:"nama_kelas"`
	InspectorName   string             `json:"inspector_name"`
	Violations      []StudentViolation `json:"violations"`
}

type UpdateInspectionInput struct {
	Tanggal         string                          `json:"tanggal"`
	JumlahDiperiksa int32                           `json:"jumlah_diperiksa"`
	Catatan         *string                         `json:"catatan"`
	Violations      []CreateInspectionViolationItem `json:"violations"`
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

type UpdateViolationInput struct {
	ViolationTypeID int64   `json:"violation_type_id"`
	Poin            *int32  `json:"poin"`
	Keterangan      *string `json:"keterangan"`
	TindakLanjut    *string `json:"tindak_lanjut"`
	Tanggal         string  `json:"tanggal"`
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

type SchoolStudent struct {
	ID           int64   `json:"id"`
	NIS          *string `json:"nis"`
	Nama         string  `json:"nama"`
	JenisKelamin string  `json:"jenis_kelamin"`
	NoHpOrtu     *string `json:"no_hp_ortu"`
	ClassID      int64   `json:"class_id"`
	NamaKelas    string  `json:"nama_kelas"`
	Tingkat      int16   `json:"tingkat"`
}

type WatchlistStudent struct {
	StudentID         int64   `json:"student_id"`
	NIS               *string `json:"nis"`
	StudentName       string  `json:"student_name"`
	NoHpOrtu          *string `json:"no_hp_ortu"`
	ClassID           *int64  `json:"class_id"`
	NamaKelas         *string `json:"nama_kelas"`
	TotalPoints       int64   `json:"total_points"`
	TotalIncidents    int64   `json:"total_incidents"`
	LastViolationDate Date    `json:"last_violation_date"`
	StatusSP          string  `json:"status_sp"`
}

type CounselingRecord struct {
	ID            int64     `json:"id"`
	SchoolID      int64     `json:"school_id"`
	StudentID     int64     `json:"student_id"`
	CounselorID   *int64    `json:"counselor_id"`
	Tanggal       Date      `json:"tanggal"`
	Topik         string    `json:"topik"`
	Catatan       *string   `json:"catatan"`
	TindakLanjut  *string   `json:"tindak_lanjut"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	NIS           *string   `json:"nis"`
	StudentName   string    `json:"student_name"`
	NamaKelas     *string   `json:"nama_kelas"`
	CounselorName *string   `json:"counselor_name"`
}

type CreateCounselingInput struct {
	StudentID    int64   `json:"student_id"`
	Tanggal      string  `json:"tanggal"`
	Topik        string  `json:"topik"`
	Catatan      *string `json:"catatan"`
	TindakLanjut *string `json:"tindak_lanjut"`
	Status       string  `json:"status"`
}

type UpdateCounselingInput struct {
	Tanggal      string  `json:"tanggal"`
	Topik        string  `json:"topik"`
	Catatan      *string `json:"catatan"`
	TindakLanjut *string `json:"tindak_lanjut"`
	Status       string  `json:"status"`
}

type ListCounselingFilter struct {
	StudentID *int64
	Status    *string
}

type ParentSummons struct {
	ID              int64      `json:"id"`
	SchoolID        int64      `json:"school_id"`
	StudentID       int64      `json:"student_id"`
	CreatedBy       *int64     `json:"created_by"`
	NomorSurat      string     `json:"nomor_surat"`
	TanggalSurat    Date       `json:"tanggal_surat"`
	JadwalPertemuan time.Time  `json:"jadwal_pertemuan"`
	Ruangan         string     `json:"ruangan"`
	Alasan          string     `json:"alasan"`
	NamaOrtu        *string    `json:"nama_ortu"`
	NoHpOrtu        *string    `json:"no_hp_ortu"`
	Status          string     `json:"status"`
	SentAt          *time.Time `json:"sent_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CatatanHasil    *string    `json:"catatan_hasil"`
	CreatedAt       time.Time  `json:"created_at"`
	NIS             *string    `json:"nis"`
	StudentName     string     `json:"student_name"`
	NamaKelas       *string    `json:"nama_kelas"`
	CreatedByName   *string    `json:"created_by_name"`
	TotalPoints     int64      `json:"total_points"`
}

type CreateParentSummonsInput struct {
	StudentID       int64   `json:"student_id"`
	NomorSurat      string  `json:"nomor_surat"`
	TanggalSurat    string  `json:"tanggal_surat"`
	JadwalPertemuan string  `json:"jadwal_pertemuan"`
	Ruangan         string  `json:"ruangan"`
	Alasan          string  `json:"alasan"`
	NamaOrtu        *string `json:"nama_ortu"`
	NoHpOrtu        *string `json:"no_hp_ortu"`
	Status          string  `json:"status"`
}

type UpdateParentSummonsInput struct {
	NomorSurat      string  `json:"nomor_surat"`
	TanggalSurat    string  `json:"tanggal_surat"`
	JadwalPertemuan string  `json:"jadwal_pertemuan"`
	Ruangan         string  `json:"ruangan"`
	Alasan          string  `json:"alasan"`
	NamaOrtu        *string `json:"nama_ortu"`
	NoHpOrtu        *string `json:"no_hp_ortu"`
	CatatanHasil    *string `json:"catatan_hasil"`
}

type ListSummonsFilter struct {
	StudentID *int64
	Status    *string
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
