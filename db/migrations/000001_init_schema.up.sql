-- Skema sistem presensi dan kedisiplinan SD Islam Bakti Asih (v4 gabungan)
-- PostgreSQL 16

-- =========================================================
-- AKUN DAN PEGAWAI
-- =========================================================
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      VARCHAR(50)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_users_username UNIQUE (username)
);

CREATE TABLE roles (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kode       VARCHAR(30) NOT NULL,
    nama       VARCHAR(60) NOT NULL,
    deskripsi  TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_roles_kode UNIQUE (kode)
);

CREATE TABLE user_roles (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id    BIGINT      NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_roles UNIQUE (user_id, role_id)
);
CREATE INDEX idx_user_roles_role_id ON user_roles (role_id);

CREATE TABLE staff (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT       REFERENCES users (id) ON DELETE SET NULL,
    nip        VARCHAR(30),
    nama       VARCHAR(100) NOT NULL,
    jabatan    VARCHAR(50),
    no_hp      VARCHAR(20),
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_staff_user_id UNIQUE (user_id),
    CONSTRAINT uq_staff_nip UNIQUE (nip)
);

-- =========================================================
-- AKADEMIK
-- =========================================================
CREATE TABLE academic_years (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nama        VARCHAR(20) NOT NULL,
    semester    VARCHAR(10) NOT NULL,
    tgl_mulai   DATE        NOT NULL,
    tgl_selesai DATE        NOT NULL,
    is_active   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_academic_years_semester CHECK (semester IN ('ganjil', 'genap')),
    CONSTRAINT chk_academic_years_tanggal CHECK (tgl_selesai > tgl_mulai),
    CONSTRAINT uq_academic_years UNIQUE (nama, semester)
);
-- hanya boleh ada satu periode aktif
CREATE UNIQUE INDEX uq_academic_years_active ON academic_years (is_active) WHERE is_active;

CREATE TABLE holidays (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year_id BIGINT       NOT NULL REFERENCES academic_years (id) ON DELETE CASCADE,
    tanggal          DATE         NOT NULL,
    keterangan       VARCHAR(150) NOT NULL,
    jenis            VARCHAR(20)  NOT NULL DEFAULT 'nasional',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_holidays_tanggal UNIQUE (tanggal),
    CONSTRAINT chk_holidays_jenis CHECK (jenis IN ('nasional', 'cuti_bersama', 'sekolah'))
);
CREATE INDEX idx_holidays_academic_year ON holidays (academic_year_id);

CREATE TABLE classes (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year_id BIGINT      NOT NULL REFERENCES academic_years (id) ON DELETE RESTRICT,
    wali_kelas_id    BIGINT      REFERENCES staff (id) ON DELETE SET NULL,
    nama_kelas       VARCHAR(20) NOT NULL,
    tingkat          SMALLINT    NOT NULL,
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_classes_tingkat CHECK (tingkat BETWEEN 1 AND 6),
    CONSTRAINT uq_classes_nama UNIQUE (academic_year_id, nama_kelas)
);
CREATE INDEX idx_classes_wali_kelas ON classes (wali_kelas_id);

CREATE TABLE subjects (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kode       VARCHAR(20)  NOT NULL,
    nama       VARCHAR(100) NOT NULL,
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_subjects_kode UNIQUE (kode)
);

CREATE TABLE class_subjects (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    class_id   BIGINT      NOT NULL REFERENCES classes (id) ON DELETE CASCADE,
    subject_id BIGINT      NOT NULL REFERENCES subjects (id) ON DELETE RESTRICT,
    teacher_id BIGINT      NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_class_subjects UNIQUE (class_id, subject_id),
    -- dipakai FK komposit dari attendance_sessions agar mapel pasti milik kelas yang sama
    CONSTRAINT uq_class_subjects_id_class UNIQUE (id, class_id)
);
CREATE INDEX idx_class_subjects_teacher ON class_subjects (teacher_id);

CREATE TABLE students (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nis           VARCHAR(20),
    nisn          VARCHAR(20),
    nama          VARCHAR(100) NOT NULL,
    jenis_kelamin CHAR(1)      NOT NULL,
    no_hp_ortu    VARCHAR(20),
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_students_jk CHECK (jenis_kelamin IN ('L', 'P')),
    CONSTRAINT uq_students_nis UNIQUE (nis),
    CONSTRAINT uq_students_nisn UNIQUE (nisn)
);

CREATE TABLE class_enrollments (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    class_id   BIGINT      NOT NULL REFERENCES classes (id) ON DELETE RESTRICT,
    student_id BIGINT      NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    status     VARCHAR(15) NOT NULL DEFAULT 'aktif',
    tgl_masuk  DATE        NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_class_enrollments_status CHECK (status IN ('aktif', 'naik_kelas', 'pindah', 'lulus', 'keluar')),
    CONSTRAINT uq_class_enrollments UNIQUE (class_id, student_id)
);
-- satu siswa hanya boleh punya satu enrollment aktif
-- (saat kenaikan kelas, ubah status lama menjadi 'naik_kelas' sebelum membuat yang baru)
CREATE UNIQUE INDEX uq_class_enrollments_active ON class_enrollments (student_id) WHERE status = 'aktif';
CREATE INDEX idx_class_enrollments_class ON class_enrollments (class_id);

-- =========================================================
-- PRESENSI PEGAWAI
-- =========================================================
CREATE TABLE qr_sessions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_by      BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    token_qr        VARCHAR(64) NOT NULL,   -- secret sesi; QR yang tampil diturunkan dari secret ini
    tanggal         DATE        NOT NULL,
    tipe_sesi       VARCHAR(10) NOT NULL,
    jam_buka        TIME        NOT NULL,
    jam_tutup       TIME        NOT NULL,
    batas_terlambat TIME,                   -- hanya relevan untuk sesi masuk
    rotasi_detik    INTEGER     NOT NULL DEFAULT 30,  -- 0 = QR tidak berganti
    status          VARCHAR(10) NOT NULL DEFAULT 'aktif',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_qr_sessions_tipe CHECK (tipe_sesi IN ('masuk', 'pulang')),
    CONSTRAINT chk_qr_sessions_status CHECK (status IN ('aktif', 'selesai')),
    CONSTRAINT chk_qr_sessions_jam CHECK (jam_tutup > jam_buka),
    CONSTRAINT chk_qr_sessions_rotasi CHECK (rotasi_detik >= 0),
    CONSTRAINT uq_qr_sessions_token UNIQUE (token_qr),
    CONSTRAINT uq_qr_sessions_tanggal_tipe UNIQUE (tanggal, tipe_sesi)
);

CREATE TABLE staff_attendances (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    staff_id          BIGINT      NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    masuk_session_id  BIGINT      REFERENCES qr_sessions (id) ON DELETE SET NULL,
    pulang_session_id BIGINT      REFERENCES qr_sessions (id) ON DELETE SET NULL,
    corrected_by      BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    tanggal           DATE        NOT NULL,
    jam_masuk         TIMESTAMPTZ,
    jam_pulang        TIMESTAMPTZ,
    status            VARCHAR(10) NOT NULL,
    metode            VARCHAR(10) NOT NULL DEFAULT 'qr',
    keterangan        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_staff_attendances_status CHECK (status IN ('hadir', 'terlambat', 'izin', 'sakit', 'alpa')),
    CONSTRAINT chk_staff_attendances_metode CHECK (metode IN ('qr', 'manual')),
    CONSTRAINT uq_staff_attendances UNIQUE (staff_id, tanggal)
);
CREATE INDEX idx_staff_attendances_tanggal ON staff_attendances (tanggal);

-- =========================================================
-- PRESENSI SISWA
-- =========================================================
CREATE TABLE attendance_sessions (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    class_id         BIGINT      NOT NULL REFERENCES classes (id) ON DELETE RESTRICT,
    class_subject_id BIGINT,
    recorded_by      BIGINT      NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    tanggal          DATE        NOT NULL,
    is_locked        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- jika class_subject_id terisi, mapel itu harus milik class_id yang sama
    CONSTRAINT fk_attendance_sessions_class_subject
        FOREIGN KEY (class_subject_id, class_id)
        REFERENCES class_subjects (id, class_id) ON DELETE RESTRICT
);
-- absen harian (tanpa mapel): satu sesi per kelas per hari
CREATE UNIQUE INDEX uq_attendance_sessions_daily
    ON attendance_sessions (class_id, tanggal) WHERE class_subject_id IS NULL;
-- absen per mapel: satu sesi per mapel per kelas per hari
CREATE UNIQUE INDEX uq_attendance_sessions_subject
    ON attendance_sessions (class_id, class_subject_id, tanggal) WHERE class_subject_id IS NOT NULL;
CREATE INDEX idx_attendance_sessions_tanggal ON attendance_sessions (tanggal);

CREATE TABLE student_attendances (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id BIGINT      NOT NULL REFERENCES attendance_sessions (id) ON DELETE CASCADE,
    student_id BIGINT      NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    status     VARCHAR(10) NOT NULL,
    keterangan TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_student_attendances_status CHECK (status IN ('hadir', 'sakit', 'izin', 'alpa')),
    CONSTRAINT uq_student_attendances UNIQUE (session_id, student_id)
);
CREATE INDEX idx_student_attendances_student ON student_attendances (student_id);

-- =========================================================
-- REKAP (snapshot setelah dikunci oleh TU)
-- =========================================================
CREATE TABLE staff_attendance_recaps (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    staff_id         BIGINT       NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    verified_by      BIGINT       REFERENCES users (id) ON DELETE SET NULL,
    bulan            SMALLINT     NOT NULL,
    tahun            SMALLINT     NOT NULL,
    total_hari_kerja INTEGER      NOT NULL DEFAULT 0,
    total_hadir      INTEGER      NOT NULL DEFAULT 0,
    total_terlambat  INTEGER      NOT NULL DEFAULT 0,
    total_izin       INTEGER      NOT NULL DEFAULT 0,
    total_sakit      INTEGER      NOT NULL DEFAULT 0,
    total_alpa       INTEGER      NOT NULL DEFAULT 0,
    persentase_hadir NUMERIC(5,2) NOT NULL DEFAULT 0,
    status_kunci     VARCHAR(10)  NOT NULL DEFAULT 'draft',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_staff_recaps_bulan CHECK (bulan BETWEEN 1 AND 12),
    CONSTRAINT chk_staff_recaps_status CHECK (status_kunci IN ('draft', 'dikunci')),
    CONSTRAINT chk_staff_recaps_total CHECK (
        total_hari_kerja >= 0 AND total_hadir >= 0 AND total_terlambat >= 0
        AND total_izin >= 0 AND total_sakit >= 0 AND total_alpa >= 0),
    CONSTRAINT uq_staff_recaps UNIQUE (staff_id, bulan, tahun)
);

CREATE TABLE student_attendance_recaps (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    student_id       BIGINT       NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    class_id         BIGINT       NOT NULL REFERENCES classes (id) ON DELETE RESTRICT,
    verified_by      BIGINT       REFERENCES users (id) ON DELETE SET NULL,
    bulan            SMALLINT     NOT NULL,
    tahun            SMALLINT     NOT NULL,
    total_hadir      INTEGER      NOT NULL DEFAULT 0,
    total_sakit      INTEGER      NOT NULL DEFAULT 0,
    total_izin       INTEGER      NOT NULL DEFAULT 0,
    total_alpa       INTEGER      NOT NULL DEFAULT 0,
    persentase_hadir NUMERIC(5,2) NOT NULL DEFAULT 0,
    status_kunci     VARCHAR(10)  NOT NULL DEFAULT 'draft',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_student_recaps_bulan CHECK (bulan BETWEEN 1 AND 12),
    CONSTRAINT chk_student_recaps_status CHECK (status_kunci IN ('draft', 'dikunci')),
    CONSTRAINT chk_student_recaps_total CHECK (
        total_hadir >= 0 AND total_sakit >= 0 AND total_izin >= 0 AND total_alpa >= 0),
    CONSTRAINT uq_student_recaps UNIQUE (student_id, class_id, bulan, tahun)
);
CREATE INDEX idx_student_recaps_class ON student_attendance_recaps (class_id, bulan, tahun);

-- =========================================================
-- KEDISIPLINAN (dilakukan oleh kesiswaan)
-- =========================================================
CREATE TABLE violation_types (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kode         VARCHAR(20)  NOT NULL,
    nama         VARCHAR(100) NOT NULL,
    kategori     VARCHAR(20)  NOT NULL,
    poin_default INTEGER      NOT NULL DEFAULT 0,
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_violation_types_kategori CHECK (kategori IN ('atribut', 'kerapihan', 'perilaku', 'lainnya')),
    CONSTRAINT chk_violation_types_poin CHECK (poin_default >= 0),
    CONSTRAINT uq_violation_types_kode UNIQUE (kode)
);

CREATE TABLE discipline_inspections (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    class_id         BIGINT      NOT NULL REFERENCES classes (id) ON DELETE RESTRICT,
    inspector_id     BIGINT      NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    tanggal          DATE        NOT NULL,
    jumlah_diperiksa INTEGER     NOT NULL DEFAULT 0,
    catatan          TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_discipline_inspections_jumlah CHECK (jumlah_diperiksa >= 0),
    CONSTRAINT uq_discipline_inspections UNIQUE (class_id, tanggal)
);
CREATE INDEX idx_discipline_inspections_tanggal ON discipline_inspections (tanggal);

CREATE TABLE student_violations (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    inspection_id     BIGINT       NOT NULL REFERENCES discipline_inspections (id) ON DELETE CASCADE,
    student_id        BIGINT       NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    violation_type_id BIGINT       NOT NULL REFERENCES violation_types (id) ON DELETE RESTRICT,
    poin              INTEGER      NOT NULL DEFAULT 0,   -- snapshot poin saat dicatat
    keterangan        TEXT,
    tindak_lanjut     VARCHAR(100),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_student_violations_poin CHECK (poin >= 0),
    CONSTRAINT uq_student_violations UNIQUE (inspection_id, student_id, violation_type_id)
);
CREATE INDEX idx_student_violations_student ON student_violations (student_id);
CREATE INDEX idx_student_violations_type ON student_violations (violation_type_id);
