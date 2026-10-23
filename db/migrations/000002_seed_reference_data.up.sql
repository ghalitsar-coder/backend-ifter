INSERT INTO roles (kode, nama, deskripsi) VALUES
    ('admin',          'Administrator',  'Kelola akun, peran, dan master data'),
    ('tu',             'Tata Usaha',     'Lihat semua presensi, koreksi absen pegawai, buat dan verifikasi rekap'),
    ('kesiswaan',      'Kesiswaan',      'Membuat pemeriksaan kedisiplinan dan mengelola jenis pelanggaran'),
    ('guru',           'Guru',           'Mengisi absen siswa pada kelas atau mapel yang diampu'),
    ('kepala_sekolah', 'Kepala Sekolah', 'Melihat seluruh data dan rekap');

-- Contoh awal; sesuaikan dengan tata tertib SD Islam Bakti Asih
INSERT INTO violation_types (kode, nama, kategori, poin_default) VALUES
    ('ATR-01', 'Seragam tidak lengkap',      'atribut',   5),
    ('ATR-02', 'Tidak memakai sepatu sesuai ketentuan', 'atribut', 5),
    ('ATR-03', 'Tidak memakai atribut sekolah',          'atribut', 3),
    ('KRP-01', 'Rambut tidak rapi',          'kerapihan', 5),
    ('KRP-02', 'Kuku panjang',               'kerapihan', 3),
    ('PRL-01', 'Tidak mengerjakan tugas',    'perilaku',  5),
    ('PRL-02', 'Mengganggu teman saat belajar', 'perilaku', 10);
