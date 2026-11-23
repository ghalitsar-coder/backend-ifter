-- 000007_discipline_counseling_summons.up.sql
-- Tabel Bimbingan Konseling (BK) & Surat Pemanggilan Orang Tua (SD Islam Bakti Asih)

CREATE TABLE counseling_records (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id     BIGINT       NOT NULL DEFAULT 1 REFERENCES schools (id) ON DELETE CASCADE,
    student_id    BIGINT       NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    counselor_id  BIGINT       REFERENCES staff (id) ON DELETE SET NULL,
    tanggal       DATE         NOT NULL DEFAULT CURRENT_DATE,
    topik         VARCHAR(200) NOT NULL,
    catatan       TEXT,
    tindak_lanjut VARCHAR(200),
    status        VARCHAR(20)  NOT NULL DEFAULT 'scheduled',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_counseling_status CHECK (status IN ('scheduled', 'ongoing', 'completed'))
);
CREATE INDEX idx_counseling_records_school ON counseling_records (school_id);
CREATE INDEX idx_counseling_records_student ON counseling_records (student_id);
CREATE INDEX idx_counseling_records_tanggal ON counseling_records (tanggal);

CREATE TABLE parent_summons (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id        BIGINT       NOT NULL DEFAULT 1 REFERENCES schools (id) ON DELETE CASCADE,
    student_id       BIGINT       NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    created_by       BIGINT       REFERENCES staff (id) ON DELETE SET NULL,
    nomor_surat      VARCHAR(80)  NOT NULL,
    tanggal_surat    DATE         NOT NULL DEFAULT CURRENT_DATE,
    jadwal_pertemuan TIMESTAMPTZ  NOT NULL,
    ruangan          VARCHAR(120) NOT NULL DEFAULT 'Ruang Bimbingan Konseling (BK)',
    alasan           TEXT         NOT NULL,
    nama_ortu        VARCHAR(100),
    no_hp_ortu       VARCHAR(25),
    status           VARCHAR(20)  NOT NULL DEFAULT 'draft',
    sent_at          TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    catatan_hasil    TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_parent_summons_status CHECK (status IN ('draft', 'sent', 'completed')),
    CONSTRAINT uq_parent_summons_school_nomor UNIQUE (school_id, nomor_surat)
);
CREATE INDEX idx_parent_summons_school ON parent_summons (school_id);
CREATE INDEX idx_parent_summons_student ON parent_summons (student_id);
CREATE INDEX idx_parent_summons_status ON parent_summons (status);
