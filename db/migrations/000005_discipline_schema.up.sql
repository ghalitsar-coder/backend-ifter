-- 000005_discipline_schema.up.sql
-- Modul Kedisiplinan Siswa (Kelompok D-03 / SD Islam Bakti Asih)
-- Multi-tenant: scoped per school_id

CREATE TABLE violation_types (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id    BIGINT       NOT NULL DEFAULT 1 REFERENCES schools (id) ON DELETE CASCADE,
    kode         VARCHAR(20)  NOT NULL,
    nama         VARCHAR(100) NOT NULL,
    kategori     VARCHAR(20)  NOT NULL,
    poin_default INTEGER      NOT NULL DEFAULT 0,
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_violation_types_kategori CHECK (kategori IN ('atribut', 'kerapihan', 'perilaku', 'lainnya')),
    CONSTRAINT chk_violation_types_poin CHECK (poin_default >= 0),
    CONSTRAINT uq_violation_types_school_kode UNIQUE (school_id, kode)
);
CREATE INDEX idx_violation_types_school ON violation_types (school_id);

CREATE TABLE discipline_inspections (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id        BIGINT      NOT NULL DEFAULT 1 REFERENCES schools (id) ON DELETE CASCADE,
    class_id         BIGINT      NOT NULL REFERENCES classes (id) ON DELETE RESTRICT,
    inspector_id     BIGINT      NOT NULL REFERENCES staff (id) ON DELETE RESTRICT,
    tanggal          DATE        NOT NULL,
    jumlah_diperiksa INTEGER     NOT NULL DEFAULT 0,
    catatan          TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_discipline_inspections_jumlah CHECK (jumlah_diperiksa >= 0),
    CONSTRAINT uq_discipline_inspections UNIQUE (class_id, tanggal)
);
CREATE INDEX idx_discipline_inspections_school ON discipline_inspections (school_id);
CREATE INDEX idx_discipline_inspections_tanggal ON discipline_inspections (tanggal);

CREATE TABLE student_violations (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id         BIGINT       NOT NULL DEFAULT 1 REFERENCES schools (id) ON DELETE CASCADE,
    inspection_id     BIGINT       REFERENCES discipline_inspections (id) ON DELETE CASCADE,
    student_id        BIGINT       NOT NULL REFERENCES students (id) ON DELETE RESTRICT,
    violation_type_id BIGINT       NOT NULL REFERENCES violation_types (id) ON DELETE RESTRICT,
    poin              INTEGER      NOT NULL DEFAULT 0,
    keterangan        TEXT,
    tindak_lanjut     VARCHAR(100),
    tanggal           DATE         NOT NULL DEFAULT CURRENT_DATE,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_student_violations_poin CHECK (poin >= 0)
);
CREATE INDEX idx_student_violations_school ON student_violations (school_id);
CREATE INDEX idx_student_violations_student ON student_violations (student_id);
CREATE INDEX idx_student_violations_type ON student_violations (violation_type_id);
CREATE INDEX idx_student_violations_tanggal ON student_violations (tanggal);

-- Data Awal Katalog Pelanggaran (Bakti Asih)
INSERT INTO violation_types (school_id, kode, nama, kategori, poin_default) VALUES
    (1, 'ATR-01', 'Seragam tidak lengkap',                'atribut',   3),
    (1, 'ATR-02', 'Tidak memakai sepatu sesuai ketentuan', 'atribut',   3),
    (1, 'ATR-03', 'Tidak memakai atribut/dasi sekolah',    'atribut',   2),
    (1, 'KRP-01', 'Rambut tidak rapi',                    'kerapihan', 3),
    (1, 'KRP-02', 'Kuku panjang / kotor',                 'kerapihan', 2),
    (1, 'PRL-01', 'Terlambat masuk sekolah',              'perilaku',  2),
    (1, 'PRL-02', 'Tidak mengerjakan tugas/PR',           'perilaku',  5),
    (1, 'PRL-03', 'Mengganggu ketertiban kelas',          'perilaku',  5),
    (1, 'PRL-04', 'Membawa barang terlarang/gadget',      'perilaku',  15),
    (1, 'PRL-05', 'Perkelahian / bullying',               'perilaku',  50)
ON CONFLICT (school_id, kode) DO NOTHING;
