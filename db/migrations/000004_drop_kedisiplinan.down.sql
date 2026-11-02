-- Rollback restore: recreate the KEDISIPLINAN section of 000001 and the
-- violation_types rows of 000002 exactly as they were.
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

INSERT INTO violation_types (kode, nama, kategori, poin_default) VALUES
    ('ATR-01', 'Seragam tidak lengkap',      'atribut',   5),
    ('ATR-02', 'Tidak memakai sepatu sesuai ketentuan', 'atribut', 5),
    ('ATR-03', 'Tidak memakai atribut sekolah',          'atribut', 3),
    ('KRP-01', 'Rambut tidak rapi',          'kerapihan', 5),
    ('KRP-02', 'Kuku panjang',               'kerapihan', 3),
    ('PRL-01', 'Tidak mengerjakan tugas',    'perilaku',  5),
    ('PRL-02', 'Mengganggu teman saat belajar', 'perilaku', 10);
