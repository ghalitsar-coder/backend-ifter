# PRODUCT REQUIREMENTS DOCUMENT (PRD)

## School Attendance &amp; Discipline Management System

**Versi:** 1.0 — MVP  
**Status:** Draft  
**Tipe Produk:** Web Application  
**Target Pengguna:** Satu sekolah  
**Platform:** Web responsif (Desktop, Tablet, Mobile)  
**Bahasa:** Bahasa Indonesia

---

# 1. Product Overview

## 1.1. Latar Belakang

Pencatatan kehadiran dan kedisiplinan siswa masih sering dilakukan secara manual menggunakan buku absensi atau spreadsheet terpisah.

Cara tersebut berpotensi menimbulkan berbagai permasalahan, seperti:

- Kesulitan merekap kehadiran harian dan bulanan.
- Kesalahan pencatatan status kehadiran.
- Kesulitan memantau keterlambatan siswa.
- Pencatatan pelanggaran kedisiplinan yang tidak terpusat.
- Kesulitan mengelola data siswa dan guru dalam jumlah besar.
- Proses pemindahan data dari Excel ke sistem yang memakan waktu.
- Kesulitan membuat laporan untuk kepala sekolah dan wali kelas.

Diperlukan aplikasi berbasis web yang dapat menyatukan pengelolaan data siswa, data guru, absensi, kedisiplinan, dan pelaporan dalam satu sistem.

## 1.2. Product Vision

Membangun aplikasi manajemen absensi dan kedisiplinan sekolah yang sederhana, efisien, mudah digunakan, dan memiliki fondasi teknis yang siap dikembangkan menjadi platform yang lebih besar.

Untuk versi MVP, aplikasi digunakan oleh satu sekolah dengan satu basis data operasional.

## 1.3. Tujuan Produk

1. Mempermudah guru dan petugas dalam mencatat kehadiran siswa.
2. Menyediakan rekap absensi yang akurat dan mudah diakses.
3. Memusatkan pencatatan pelanggaran kedisiplinan siswa.
4. Mempermudah pengelolaan data siswa dan guru melalui Excel.
5. Mengurangi pekerjaan administratif yang berulang.
6. Menyediakan laporan yang dapat diekspor untuk kebutuhan sekolah.
7. Membangun fondasi aplikasi yang memungkinkan penambahan modul baru pada masa mendatang.

## 1.4. Scope MVP

MVP berfokus pada empat modul:

1. **Dashboard &amp; Ringkasan**
2. **Manajemen Data Siswa dan Guru**
3. **Manajemen Absensi**
4. **Manajemen Kedisiplinan**

Fitur pendukung:

- Autentikasi dan hak akses pengguna.
- Import dan export Excel.
- Filter, pencarian, dan pagination.
- Laporan absensi dan kedisiplinan.
- Audit pencatatan dan perubahan data penting.

---

# 2. Target Pengguna dan Hak Akses

MVP menggunakan Role-Based Access Control (RBAC) dengan tiga peran utama.

## 2.1. Administrator Sekolah

Bertanggung jawab atas pengelolaan sistem secara keseluruhan.

Hak akses:

- Melihat dashboard seluruh sekolah.
- Mengelola data siswa.
- Mengelola data guru.
- Mengelola kelas.
- Mengatur tahun ajaran dan semester.
- Mengatur aturan absensi.
- Mengelola kategori kedisiplinan.
- Mengakses seluruh laporan.
- Melakukan import dan export Excel.
- Mengelola akun pengguna.
- Mengoreksi data absensi dengan alasan yang tercatat.

## 2.2. Guru / Wali Kelas

Bertanggung jawab atas pencatatan kehadiran dan pemantauan siswa sesuai kewenangannya.

Hak akses:

- Melihat dashboard kelas yang menjadi tanggung jawabnya.
- Melihat daftar siswa di kelasnya.
- Mencatat absensi harian.
- Memperbarui status kehadiran sesuai kebijakan sekolah.
- Melihat riwayat absensi kelas.
- Mencatat pelanggaran kedisiplinan siswa.
- Melihat riwayat kedisiplinan siswa di kelasnya.
- Membuat laporan kelas.

Guru tidak boleh mengelola akun administrator atau mengubah data sekolah secara global.

## 2.3. Guru Piket

Bertanggung jawab atas pemantauan kehadiran dan kedisiplinan harian.

Hak akses:

- Melihat daftar siswa dari seluruh kelas yang diperlukan untuk tugas piket.
- Mencatat kehadiran siswa.
- Mencatat keterlambatan.
- Melihat daftar siswa yang belum tercatat kehadirannya.
- Mencatat pelanggaran atribut dan kedisiplinan.
- Melihat ringkasan absensi harian.

Hak akses guru piket dapat dibatasi agar tidak dapat mengubah data master siswa, guru, dan kelas.

**Catatan:** satu orang dapat memiliki lebih dari satu peran jika sekolah mengizinkannya. Hak akses tetap ditentukan oleh sistem, bukan hanya oleh tampilan menu.

---

# 3. Functional Requirements

## MODULE A — Authentication &amp; User Management

### FR-A01: Login

Pengguna dapat masuk menggunakan:

- Username atau email.
- Password.

Sistem harus:

- Memvalidasi kredensial.
- Menolak login dengan kredensial tidak valid.
- Mengelola sesi pengguna secara aman.
- Mengarahkan pengguna ke dashboard sesuai hak akses.

### FR-A02: User Management

Administrator dapat:

- Membuat akun guru.
- Menetapkan peran pengguna.
- Mengaktifkan atau menonaktifkan akun.
- Mengatur ulang akses pengguna melalui mekanisme yang aman.

### FR-A03: Access Control

Sistem wajib memvalidasi hak akses pada backend untuk setiap operasi yang dilindungi.

Pengguna yang tidak berwenang tidak boleh dapat mengakses data hanya dengan mengganti URL atau parameter request.

---

## MODULE B — Dashboard

### FR-B01: Dashboard Administrator

Menampilkan ringkasan operasional sekolah.

Komponen:

- Total siswa aktif.
- Total guru aktif.
- Jumlah siswa hadir hari ini.
- Jumlah siswa terlambat.
- Jumlah siswa izin.
- Jumlah siswa sakit.
- Jumlah siswa tanpa keterangan.
- Jumlah catatan pelanggaran pada periode berjalan.
- Grafik tren kehadiran.
- Ringkasan distribusi status kehadiran.
- Daftar aktivitas terbaru.

Dashboard harus memiliki filter tanggal dan, jika relevan, kelas.

### FR-B02: Dashboard Guru

Menampilkan informasi yang relevan dengan kelas atau tugas guru.

Komponen:

- Jumlah siswa di kelas.
- Ringkasan kehadiran hari ini.
- Daftar siswa yang belum diabsen.
- Daftar siswa terlambat.
- Ringkasan pelanggaran kedisiplinan.
- Akses cepat menuju pencatatan absensi.

### FR-B03: Dashboard Guru Piket

Menampilkan:

- Rekap kehadiran harian.
- Daftar siswa terlambat.
- Daftar siswa yang belum memiliki catatan kehadiran.
- Catatan pelanggaran yang dibuat hari itu.

Semua angka pada dashboard harus berasal dari data aktual, bukan nilai statis.

---

## MODULE C — Student Management

### FR-C01: Data Siswa

Administrator dapat menambah, melihat, mengubah, dan mengarsipkan data siswa.

Data minimal:


| Field         | Keterangan                                                    |
| ------------- | ------------------------------------------------------------- |
| NIS           | Nomor induk siswa                                             |
| NISN          | Nomor induk siswa nasional, opsional sesuai kebutuhan sekolah |
| Nama lengkap  | Nama siswa                                                    |
| Jenis kelamin | Pilihan sesuai kebutuhan administrasi                         |
| Tanggal lahir | Opsional                                                      |
| Kelas         | Kelas aktif                                                   |
| Tahun masuk   | Tahun masuk sekolah                                           |
| Status        | Aktif, lulus, pindah, atau nonaktif                           |
| Nama wali     | Opsional                                                      |
| Kontak wali   | Opsional                                                      |


Data sensitif yang tidak diperlukan untuk absensi tidak perlu dikumpulkan pada MVP.

### FR-C02: Student List

Halaman daftar siswa menyediakan:

- Pencarian berdasarkan nama atau NIS.
- Filter berdasarkan kelas.
- Filter berdasarkan status.
- Pagination.
- Pengurutan berdasarkan nama atau NIS.
- Aksi melihat detail dan mengubah data.

### FR-C03: Student Detail

Halaman detail siswa menampilkan:

- Informasi identitas.
- Kelas saat ini.
- Ringkasan kehadiran.
- Riwayat absensi.
- Riwayat pelanggaran.
- Catatan administratif yang relevan.

Riwayat siswa tidak boleh hilang hanya karena siswa pindah kelas atau statusnya berubah menjadi nonaktif.

### FR-C04: Class Management

Administrator dapat:

- Membuat kelas.
- Mengubah nama kelas.
- Menetapkan tingkat atau jenjang.
- Menetapkan wali kelas.
- Melihat daftar siswa.
- Mengarsipkan kelas yang sudah tidak aktif.

Data kelas perlu memiliki konteks tahun ajaran agar perpindahan kelas tidak merusak riwayat historis.

---

## MODULE D — Teacher Management

### FR-D01: Data Guru

Administrator dapat mengelola data guru.

Data minimal:


| Field                | Keterangan                                           |
| -------------------- | ---------------------------------------------------- |
| NIP                  | Nomor induk pegawai, jika tersedia                   |
| Nama lengkap         | Nama guru                                            |
| Email                | Opsional, tetapi diperlukan jika dipakai untuk login |
| Nomor telepon        | Opsional                                             |
| Status               | Aktif atau nonaktif                                  |
| Peran                | Administrator, guru, wali kelas, atau guru piket     |
| Kelas tanggung jawab | Sesuai penugasan                                     |


Data guru dan akun login harus memiliki hubungan yang jelas, tetapi keduanya tidak harus menjadi satu entitas database.

### FR-D02: Teacher Assignment

Administrator dapat:

- Menetapkan wali kelas.
- Mengubah penugasan guru.
- Menetapkan guru piket.
- Melihat daftar kelas yang menjadi tanggung jawab guru.

Riwayat penugasan sebaiknya disimpan agar laporan historis tetap dapat ditelusuri.

---

## MODULE E — Attendance Management

Modul ini merupakan fitur inti aplikasi.

### FR-E01: Daily Attendance

Guru dapat mencatat kehadiran siswa berdasarkan tanggal dan kelas.

Status kehadiran MVP:


| Status           | Keterangan                             |
| ---------------- | -------------------------------------- |
| Hadir            | Siswa hadir sesuai ketentuan sekolah   |
| Terlambat        | Siswa datang melewati batas waktu      |
| Sakit            | Siswa tidak hadir karena sakit         |
| Izin             | Siswa tidak hadir dengan izin          |
| Tanpa Keterangan | Tidak hadir tanpa alasan yang tercatat |


Status tersebut dapat disesuaikan melalui konfigurasi pada pengembangan berikutnya.

### FR-E02: Attendance Workflow

Alur pencatatan:

1. Guru memilih tanggal.
2. Guru memilih kelas.
3. Sistem menampilkan daftar siswa aktif pada kelas tersebut.
4. Guru menentukan status kehadiran setiap siswa.
5. Guru menyimpan catatan.
6. Sistem memvalidasi data.
7. Sistem menyimpan catatan ke database.
8. Dashboard dan laporan diperbarui.

Sistem harus menampilkan status pencatatan, misalnya jumlah siswa yang sudah dan belum diabsen.

### FR-E03: Bulk Attendance

Guru dapat mencatat kehadiran untuk banyak siswa dalam satu halaman.

Fitur:

- Menandai beberapa siswa sebagai hadir.
- Mengubah status siswa tertentu.
- Menambahkan catatan jika diperlukan.
- Menyimpan seluruh perubahan secara terkontrol.
- Memberikan konfirmasi sebelum perubahan massal diterapkan.

Perubahan massal tidak boleh menghapus catatan yang sudah tersimpan tanpa konfirmasi.

### FR-E04: Attendance Rules

Administrator dapat mengatur:

- Jam masuk sekolah.
- Batas waktu keterlambatan.
- Hari operasional sekolah.
- Tahun ajaran aktif.
- Kalender hari libur.

Pada MVP, keterlambatan dapat ditentukan berdasarkan waktu kedatangan yang dicatat guru atau petugas.

Sistem tidak boleh menganggap siswa tanpa catatan absensi sebagai pasti tidak hadir sebelum proses pencatatan hari tersebut selesai atau aturan penutupan absensi diterapkan.

### FR-E05: Attendance History

Pengguna dapat melihat riwayat absensi berdasarkan:

- Nama siswa.
- NIS.
- Kelas.
- Tanggal.
- Bulan.
- Tahun ajaran.
- Status kehadiran.

Riwayat dapat diakses dari halaman absensi dan detail siswa sesuai hak akses.

### FR-E06: Attendance Correction

Administrator atau pengguna yang diberi kewenangan dapat memperbaiki catatan absensi.

Setiap koreksi harus menyimpan:

- Data sebelum perubahan.
- Data setelah perubahan.
- Pengguna yang melakukan perubahan.
- Waktu perubahan.
- Alasan koreksi.

Perubahan tidak boleh dilakukan secara diam-diam tanpa jejak audit.

### FR-E07: Attendance Summary

Sistem menghitung ringkasan kehadiran siswa untuk periode tertentu.

Contoh:

- Total hadir.
- Total terlambat.
- Total sakit.
- Total izin.
- Total tanpa keterangan.
- Persentase kehadiran.

Rumus dan denominator persentase harus didefinisikan secara konsisten. Hari libur, hari sekolah, dan status siswa perlu diperhitungkan agar persentase tidak menyesatkan.

### FR-E08: Attendance Report

Administrator dapat membuat laporan:

- Harian.
- Mingguan.
- Bulanan.
- Per siswa.
- Per kelas.
- Per tahun ajaran.

Laporan dapat difilter sebelum diekspor.

---

## MODULE F — Discipline Management

Modul ini digunakan untuk mencatat pelanggaran dan memantau tindak lanjut kedisiplinan siswa.

### FR-F01: Violation Categories

Administrator dapat mengelola kategori pelanggaran.

Contoh kategori:

- Terlambat.
- Tidak memakai atribut lengkap.
- Seragam tidak sesuai ketentuan.
- Pelanggaran tata tertib kelas.
- Pelanggaran tata tertib sekolah.
- Pelanggaran lainnya.

Kategori harus dapat ditambah, diubah, dan dinonaktifkan sesuai kebijakan sekolah.

### FR-F02: Violation Recording

Guru atau guru piket dapat mencatat pelanggaran.

Data yang dicatat:

- Siswa.
- Tanggal dan waktu kejadian.
- Kategori pelanggaran.
- Deskripsi kejadian.
- Petugas pencatat.
- Poin pelanggaran, jika kebijakan sekolah menggunakannya.
- Catatan tindak lanjut.
- Status penanganan.

Unggahan foto bukti tidak diwajibkan pada MVP dan dapat ditambahkan kemudian jika memang diperlukan.

### FR-F03: Discipline Points

Sekolah dapat menerapkan sistem poin pelanggaran.

Contoh konfigurasi:


| Pelanggaran             | Poin |
| ----------------------- | ---- |
| Terlambat               | 2    |
| Atribut tidak lengkap   | 3    |
| Seragam tidak sesuai    | 3    |
| Pelanggaran tata tertib | 5    |


Angka di atas hanya ilustrasi.

Administrator menentukan bobot poin sesuai peraturan sekolah.

Sistem menghitung total poin dari catatan pelanggaran yang masih berlaku.

Jika sebuah catatan dikoreksi atau dibatalkan, perhitungan harus ikut diperbarui tanpa menghapus riwayat perubahan.

### FR-F04: Discipline History

Halaman detail kedisiplinan siswa menampilkan:

- Total poin pelanggaran.
- Jumlah pelanggaran berdasarkan kategori.
- Riwayat kejadian.
- Catatan tindak lanjut.
- Status penyelesaian.

### FR-F05: Discipline Follow-up

Setiap pelanggaran dapat memiliki status:

- Baru dicatat.
- Dalam peninjauan.
- Dalam tindak lanjut.
- Selesai.
- Dibatalkan setelah verifikasi.

Petugas yang berwenang dapat memperbarui status dengan menyertakan catatan jika diperlukan.

### FR-F06: Discipline Report

Laporan dapat difilter berdasarkan:

- Periode.
- Siswa.
- Kelas.
- Kategori pelanggaran.
- Petugas pencatat.
- Status penanganan.

Laporan dapat diekspor ke Excel.

### FR-F07: Discipline Access Control

Catatan kedisiplinan hanya dapat diakses oleh pihak yang berwenang.

Sistem tidak boleh menampilkan daftar pelanggaran individual kepada seluruh siswa atau publik.

Catatan harus dapat dikoreksi apabila terjadi kesalahan pencatatan.

---

## MODULE G — Excel Import &amp; Export

Modul ini merupakan salah satu fitur wajib MVP.

Import dan export harus mendukung data siswa, data guru, dan laporan absensi serta kedisiplinan.

### FR-G01: Student Import

Administrator dapat mengunggah file Excel untuk memasukkan data siswa secara massal.

Alur:

1. Unduh template Excel.
2. Isi data siswa.
3. Unggah file.
4. Sistem memvalidasi format dan kolom.
5. Sistem memeriksa data wajib.
6. Sistem memeriksa duplikasi NIS.
7. Sistem memvalidasi referensi kelas.
8. Sistem menampilkan preview.
9. Administrator mengonfirmasi proses import.
10. Sistem memasukkan data yang valid.
11. Sistem menampilkan ringkasan hasil.

Contoh hasil:


| Hasil            | Jumlah |
| ---------------- | ------ |
| Total baris      | 500    |
| Berhasil         | 480    |
| Duplikat         | 10     |
| Data tidak valid | 10     |


Angka hanya ilustrasi.

Sistem harus memberikan alasan kesalahan per baris agar pengguna dapat memperbaiki file.

### FR-G02: Teacher Import

Administrator dapat memasukkan data guru melalui template Excel.

Validasi meliputi:

- Nama wajib tersedia.
- NIP tidak boleh duplikat jika digunakan sebagai identitas unik.
- Email harus valid jika diisi.
- Data penugasan harus mengacu pada kelas yang tersedia.
- Peran pengguna harus berasal dari daftar yang diizinkan.

Import data guru tidak boleh otomatis membuat akun login dengan password yang sama untuk semua pengguna.

Pembuatan akun dan pengaturan kredensial harus melalui proses yang aman.

### FR-G03: Import Strategy

Sistem menyediakan dua mode:

**Create Only**

- Hanya menambahkan data baru.
- Tidak menimpa data yang sudah ada.
- Baris duplikat ditandai.

**Upsert / Update Existing**

- Menambahkan data baru.
- Memperbarui data yang sudah ada berdasarkan identitas unik.
- Menampilkan preview perubahan sebelum diproses.
- Memerlukan konfirmasi administrator.

Mode update tidak boleh menimpa kolom penting secara tidak sengaja ketika nilai Excel kosong.

### FR-G04: Attendance Export

Administrator atau guru yang berwenang dapat mengekspor laporan absensi ke Excel.

Pilihan:

- Rekap harian.
- Rekap bulanan.
- Rekap per siswa.
- Rekap per kelas.
- Rekap per tahun ajaran.

Kolom laporan dapat mencakup:

- NIS.
- Nama siswa.
- Kelas.
- Tanggal.
- Status kehadiran.
- Waktu kedatangan jika tersedia.
- Catatan.

### FR-G05: Discipline Export

Data kedisiplinan dapat diekspor dengan kolom:

- NIS.
- Nama siswa.
- Kelas.
- Tanggal kejadian.
- Kategori.
- Deskripsi.
- Poin.
- Petugas pencatat.
- Status penanganan.

### FR-G06: Teacher Export

Administrator dapat mengekspor daftar guru berdasarkan status, penugasan, atau peran.

### FR-G07: Import Validation &amp; Error Handling

Sistem harus:

- Membatasi format file yang diterima.
- Membatasi ukuran file.
- Memvalidasi struktur kolom.
- Memvalidasi tipe data.
- Mendeteksi duplikasi.
- Menangani baris kosong.
- Menyediakan laporan kesalahan.
- Mencegah proses import parsial yang tidak disengaja.

Untuk import berskala kecil, sistem dapat memvalidasi seluruh file terlebih dahulu sebelum menyimpan data dalam satu transaksi.

Jika mode import parsial disediakan, pengguna harus diberi tahu bahwa hanya baris valid yang akan diproses.

### FR-G08: Template Management

Template Excel disediakan untuk:

- Data siswa.
- Data guru.
- Data kelas jika diperlukan.

Template harus memiliki nama kolom yang konsisten, petunjuk pengisian, dan contoh format yang valid.

**Catatan implementasi:** gunakan identitas unik yang stabil. Nama siswa tidak boleh menjadi satu-satunya kunci pencocokan data.

---

## MODULE H — Reporting

### FR-H01: Attendance Report

Sistem dapat menghasilkan laporan kehadiran berdasarkan periode dan kelas.

### FR-H02: Discipline Report

Sistem dapat menghasilkan laporan pelanggaran dan tindak lanjut.

### FR-H03: Report Filters

Pengguna dapat mengatur filter sebelum laporan dibuat.

### FR-H04: Export Format

Format wajib MVP:

- XLSX.

Format PDF dapat ditambahkan sebagai fitur pendukung jika waktu pengembangan memungkinkan.

### FR-H05: Data Accuracy

Angka dalam laporan harus dihitung dari sumber data yang sama dengan dashboard.

Sistem tidak boleh menghasilkan angka berbeda antara halaman rekap dan file ekspor untuk filter yang identik.

---

# 4. Non-Functional Requirements

## NFR-01: Security

- Password disimpan menggunakan hashing yang aman.
- Semua endpoint yang memerlukan autentikasi harus dilindungi.
- Otorisasi dilakukan di backend.
- Validasi input diterapkan pada seluruh proses.
- Proteksi terhadap SQL injection dan XSS.
- Sesi login dikelola secara aman.
- Aksi penting dicatat dalam audit log.
- Data sensitif tidak ditampilkan kepada pengguna yang tidak berwenang.

## NFR-02: Performance

Target awal untuk lingkungan penggunaan sekolah:

- Halaman daftar siswa menampilkan data menggunakan pagination.
- Operasi baca umum ditargetkan memiliki waktu respons di bawah 2 detik pada kondisi normal.
- Penyimpanan absensi massal harus memberikan konfirmasi hasil yang jelas.
- Import file besar tidak boleh membuat halaman web membeku.
- Query laporan harus menggunakan filter dan indeks yang sesuai.

Angka tersebut merupakan target awal yang perlu diuji menggunakan data dan infrastruktur nyata.

## NFR-03: Reliability

- Data absensi tidak boleh hilang akibat navigasi halaman.
- Sistem memberikan konfirmasi setelah penyimpanan berhasil.
- Kegagalan import tidak boleh menghasilkan data duplikat tanpa terdeteksi.
- Proses koreksi data harus dapat ditelusuri.
- Backup database harus tersedia dan proses pemulihannya diuji.

## NFR-04: Usability

- Antarmuka menggunakan Bahasa Indonesia.
- Pencatatan absensi harus mudah dilakukan dengan sedikit interaksi.
- Formulir memberikan pesan kesalahan yang jelas.
- Tabel mendukung pencarian, filter, dan pagination.
- Halaman dapat digunakan pada desktop dan tablet.
- Tampilan mobile tetap dapat digunakan untuk tugas utama guru.

## NFR-05: Privacy

- Pengumpulan data dibatasi pada kebutuhan operasional sekolah.
- Akses data siswa mengikuti kewenangan pengguna.
- Ekspor data harus dibatasi sesuai hak akses.
- Pengelolaan data mengikuti kebijakan retensi.
- Implementasi harus memperhatikan ketentuan perlindungan data pribadi yang berlaku di Indonesia, termasuk ketentuan terkait data anak.

## NFR-06: Maintainability

- Kode diorganisasikan berdasarkan domain.
- Aturan absensi tidak dicampur dengan logika tampilan.
- Logika import dan export dipisahkan dari halaman UI.
- Skema database mendukung riwayat akademik.
- Penambahan modul baru tidak mengharuskan perubahan besar pada modul inti.

---

# 5. Rekomendasi Data Model

Struktur berikut adalah rancangan awal, bukan skema final.

## 5.1. Entitas Utama

### users

Menyimpan akun login.

Field:

- id
- name
- email
- password_hash
- is_active
- created_at
- updated_at

### roles

Menyimpan jenis peran pengguna.

Field:

- id
- name

### user_roles

Menghubungkan pengguna dengan perannya.

Field:

- user_id
- role_id

### teachers

Menyimpan profil guru.

Field:

- id
- user_id, nullable jika belum memiliki akun
- employee_number
- full_name
- email
- phone
- status

### students

Menyimpan profil siswa.

Field:

- id
- student_number
- national_student_number, nullable
- full_name
- gender, jika diperlukan
- birth_date, nullable
- admission_year
- status
- created_at
- updated_at

### academic_years

Menyimpan tahun ajaran.

Field:

- id
- name
- start_date
- end_date
- is_active

### classes

Menyimpan identitas kelas.

Field:

- id
- name
- grade_level
- status

### class_enrollments

Menghubungkan siswa dengan kelas berdasarkan tahun ajaran.

Field:

- id
- student_id
- class_id
- academic_year_id
- start_date
- end_date, nullable
- status

Tabel ini penting agar perpindahan kelas tidak menghapus riwayat siswa.

### homeroom_assignments

Menyimpan penugasan wali kelas.

Field:

- id
- teacher_id
- class_id
- academic_year_id
- start_date
- end_date, nullable

### attendance_records

Menyimpan catatan kehadiran.

Field:

- id
- student_id
- class_id
- academic_year_id
- attendance_date
- status
- check_in_time, nullable
- notes, nullable
- recorded_by
- created_at
- updated_at

Sistem perlu menerapkan aturan unik agar satu siswa tidak memiliki lebih dari satu catatan absensi harian untuk konteks absensi yang sama.

Jika nantinya diperlukan absensi per mata pelajaran, model dapat diperluas dengan entitas attendance_sessions.

### discipline_categories

Menyimpan kategori pelanggaran.

Field:

- id
- name
- description
- default_points
- is_active

### discipline_records

Menyimpan catatan pelanggaran.

Field:

- id
- student_id
- class_id
- category_id
- occurrence_at
- description
- points
- status
- recorded_by
- created_at
- updated_at

### discipline_followups

Menyimpan riwayat tindak lanjut.

Field:

- id
- discipline_record_id
- status
- notes
- updated_by
- created_at

### audit_logs

Menyimpan jejak perubahan penting.

Field:

- id
- actor_user_id
- action
- entity_type
- entity_id
- old_values, jika relevan
- new_values, jika relevan
- created_at

### import_jobs

Menyimpan riwayat proses import.

Field:

- id
- entity_type
- file_name
- import_mode
- status
- total_rows
- successful_rows
- failed_rows
- created_by
- created_at
- completed_at, nullable

Detail kesalahan import dapat disimpan dalam tabel terpisah atau berkas hasil validasi.

## 5.2. Prinsip Database

- Gunakan primary key yang stabil.
- Terapkan foreign key untuk menjaga integritas relasi.
- Tambahkan unique constraint pada identitas yang memang unik.
- Gunakan transaksi untuk perubahan yang saling berkaitan.
- Simpan waktu menggunakan tipe data yang sesuai dan konsisten.
- Bedakan pengarsipan dengan penghapusan permanen.
- Jangan menghapus riwayat absensi ketika siswa dinonaktifkan.
- Siapkan struktur agar identitas sekolah dapat ditambahkan pada pengembangan multi-tenant mendatang.

Untuk MVP, tidak perlu langsung membangun database terpisah bagi setiap sekolah.

---

# 6. Rancangan Halaman Aplikasi

## 6.1. Dashboard

URL konseptual: `/dashboard`

Komponen:

- Statistik kehadiran.
- Grafik tren kehadiran.
- Ringkasan kedisiplinan.
- Aktivitas terbaru.
- Filter tanggal.

## 6.2. Data Siswa

URL konseptual: `/students`

Komponen:

- Tabel siswa.
- Pencarian.
- Filter kelas.
- Filter status.
- Tombol tambah siswa.
- Tombol import Excel.
- Tombol export Excel.
- Aksi detail dan edit.

## 6.3. Detail Siswa

URL konseptual: `/students/:id`

Tab:

- Profil.
- Riwayat absensi.
- Riwayat kedisiplinan.

## 6.4. Data Guru

URL konseptual: `/teachers`

Komponen:

- Tabel guru.
- Pencarian.
- Filter status.
- Tombol tambah guru.
- Tombol import Excel.
- Tombol export Excel.
- Aksi edit.

## 6.5. Data Kelas

URL konseptual: `/classes`

Komponen:

- Daftar kelas.
- Jumlah siswa.
- Wali kelas.
- Tahun ajaran.
- Aksi detail dan edit.

## 6.6. Absensi Harian

URL konseptual: `/attendance`

Komponen:

- Pemilih tanggal.
- Pemilih kelas.
- Ringkasan jumlah siswa.
- Tabel siswa.
- Pilihan status kehadiran.
- Input catatan.
- Tombol tandai hadir.
- Tombol simpan.
- Indikator siswa yang belum dicatat.

## 6.7. Riwayat Absensi

URL konseptual: `/attendance/history`

Komponen:

- Filter tanggal.
- Filter siswa.
- Filter kelas.
- Filter status.
- Tabel riwayat.
- Export Excel.

## 6.8. Kedisiplinan

URL konseptual: `/discipline`

Komponen:

- Ringkasan pelanggaran.
- Filter periode dan kelas.
- Tabel catatan pelanggaran.
- Tombol tambah pelanggaran.
- Filter kategori.
- Filter status penanganan.
- Export Excel.

## 6.9. Detail Kedisiplinan Siswa

URL konseptual: `/discipline/students/:id`

Komponen:

- Total poin.
- Riwayat pelanggaran.
- Kategori pelanggaran.
- Status tindak lanjut.
- Catatan pembinaan.

## 6.10. Laporan

URL konseptual: `/reports`

Tab:

- Laporan absensi.
- Laporan kedisiplinan.

Komponen:

- Filter periode.
- Filter kelas.
- Preview laporan.
- Export Excel.

## 6.11. Pengaturan

URL konseptual: `/settings`

Submenu:

- Profil sekolah.
- Tahun ajaran.
- Kelas.
- Aturan absensi.
- Kategori pelanggaran.
- Manajemen akun.
- Pengaturan dasar aplikasi.

---

# 7. Batasan MVP

Fitur berikut tidak termasuk dalam tahap MVP:

1. Face recognition.
2. Computer vision untuk pemeriksaan atribut.
3. AI chatbot.
4. AI analytics.
5. Prediksi kehadiran.
6. Aplikasi mobile native.
7. Portal orang tua.
8. Push notification.
9. Absensi RFID.
10. Absensi berbasis geofencing.
11. Integrasi WhatsApp.
12. Integrasi sistem akademik eksternal.
13. Subscription dan billing SaaS.
14. Multi-tenant aktif untuk banyak sekolah.
15. Absensi per mata pelajaran.
16. GPS tracking.
17. Payroll guru.
18. Manajemen nilai dan rapor.
19. Sistem perpustakaan.
20. Manajemen keuangan sekolah.

Batasan tersebut membantu menjaga ruang lingkup MVP tetap terkontrol.

---

# 8. Acceptance Criteria

MVP dianggap siap diuji jika seluruh kriteria utama berikut terpenuhi.

## Authentication

- Pengguna dapat login dan logout.
- Pengguna tanpa izin tidak dapat mengakses fitur terlarang.
- Akun nonaktif tidak dapat login.

## Student Management

- Administrator dapat membuat, mengubah, dan mengarsipkan siswa.
- Pencarian siswa berfungsi.
- Data siswa dapat diimpor dari template Excel.
- Duplikasi identitas unik dapat dideteksi.
- Riwayat siswa tetap tersedia setelah siswa diarsipkan.

## Teacher Management

- Administrator dapat membuat dan mengubah data guru.
- Penugasan wali kelas dapat dikelola.
- Data guru dapat diimpor dan diekspor.

## Attendance

- Guru dapat mencatat kehadiran siswa berdasarkan kelas dan tanggal.
- Sistem dapat mencatat seluruh status kehadiran MVP.
- Sistem mencegah catatan ganda untuk konteks absensi yang sama.
- Koreksi absensi tercatat dalam audit log.
- Rekap harian dan bulanan dapat ditampilkan.
- Laporan absensi dapat diekspor ke Excel.

## Discipline

- Pengguna berwenang dapat mencatat pelanggaran.
- Kategori pelanggaran dapat dikelola.
- Total poin dapat dihitung secara konsisten.
- Riwayat tindak lanjut dapat dilihat.
- Catatan kedisiplinan dapat difilter dan diekspor.

## Excel

- Template import tersedia.
- File dengan format tidak sesuai ditolak dengan pesan yang jelas.
- Kesalahan import ditampilkan per baris.
- Pengguna dapat melihat preview sebelum konfirmasi.
- Proses import tidak membuat duplikasi yang tidak terdeteksi.
- Export mengikuti filter yang dipilih pengguna.

## Reporting

- Data laporan sesuai dengan database.
- Filter kelas dan periode bekerja.
- Pengguna hanya dapat mengekspor data yang menjadi kewenangannya.

## Reliability

- Operasi penting memberikan umpan balik keberhasilan atau kegagalan.
- Data yang sudah tersimpan tidak hilang ketika pengguna berpindah halaman.
- Backup database dan prosedur pemulihan telah diuji.

---

# 9. Development Roadmap

## Sprint 1 — Project Foundation

- Inisialisasi proyek.
- Setup database.
- Authentication.
- RBAC.
- Layout dashboard.
- Konfigurasi dasar aplikasi.

## Sprint 2 — Master Data

- CRUD siswa.
- CRUD guru.
- CRUD kelas.
- Tahun ajaran.
- Penugasan wali kelas.

## Sprint 3 — Excel Import &amp; Export

- Template siswa.
- Import siswa.
- Validasi dan deteksi duplikasi.
- Template guru.
- Import guru.
- Export data master.

## Sprint 4 — Attendance

- Absensi harian.
- Bulk attendance.
- Riwayat absensi.
- Aturan keterlambatan.
- Koreksi absensi.
- Rekap kehadiran.

## Sprint 5 — Discipline

- Kategori pelanggaran.
- Pencatatan pelanggaran.
- Perhitungan poin.
- Tindak lanjut.
- Riwayat kedisiplinan.

## Sprint 6 — Reporting &amp; Hardening

- Dashboard final.
- Laporan absensi.
- Laporan kedisiplinan.
- Export laporan.
- Audit log.
- Pengujian hak akses.
- Pengujian import dan export.
- Pengujian performa.
- Pengujian backup dan restore.

Urutan sprint dapat disesuaikan dengan jumlah pengembang dan teknologi yang digunakan.

---

# 10. Definition of Done

Sebuah fitur dinyatakan selesai jika:

- Fungsinya berjalan sesuai kebutuhan.
- Validasi input tersedia.
- Hak akses telah diuji.
- Penanganan error tersedia.
- Data tersimpan secara konsisten.
- Tampilan dapat digunakan pada desktop dan tablet.
- Pengujian untuk alur utama tersedia.
- Tidak terdapat masalah kritis yang diketahui.
- Dokumentasi penggunaan atau teknis diperbarui jika diperlukan.

---

# 11. Future Expansion Strategy

Meskipun MVP hanya digunakan oleh satu sekolah, struktur aplikasi sebaiknya dipisahkan berdasarkan domain.

Contoh modul:

- Authentication
- School Configuration
- Student Management
- Teacher Management
- Academic Structure
- Attendance
- Discipline
- Import &amp; Export
- Reporting
- Audit &amp; Security

Fitur masa depan yang dapat ditambahkan tanpa mengubah visi utama produk:

**Tahap berikutnya:**

- Portal orang tua.
- Pengajuan izin digital.
- Notifikasi otomatis.
- Absensi per mata pelajaran.
- Pemeriksaan atribut yang lebih lengkap.
- Manajemen perangkat.

**Tahap lanjutan:**

- Offline attendance.
- API dan webhook.
- Subscription management.
- Multi-tenant.
- Analitik lanjutan.
- Integrasi eksternal.

**Tahap eksperimental:**

- Computer vision.
- Face recognition.
- AI analytics.

Fitur-fitur tersebut tidak perlu dibuat sekarang. Yang perlu dipersiapkan adalah struktur kode, skema data, dan pemisahan tanggung jawab yang tidak menyulitkan pengembangan berikutnya.

---

# 12. MVP Success Metrics

Keberhasilan MVP dinilai berdasarkan hasil pengujian operasional, bukan jumlah fitur.

Target awal:

- 100% catatan absensi yang berhasil disimpan dapat ditemukan kembali.
- Tidak ada akses lintas peran yang tidak diizinkan pada skenario pengujian.
- Import siswa dan guru menghasilkan ringkasan validasi yang benar.
- Rekap absensi konsisten dengan catatan sumber.
- Pengguna dapat menyelesaikan pencatatan absensi kelas tanpa langkah yang tidak diperlukan.
- Data kedisiplinan dapat ditelusuri hingga pengguna yang mencatatnya.
- Backup database dapat dipulihkan melalui prosedur yang terdokumentasi.

Target waktu operasional dan performa dapat ditetapkan lebih presisi setelah jumlah siswa, guru, serta kapasitas server diketahui.

---

# 13. Final Product Definition

MVP adalah aplikasi web untuk satu sekolah yang menyediakan:

1. Login dan manajemen hak akses.
2. Dashboard operasional.
3. Manajemen data siswa.
4. Manajemen data guru.
5. Manajemen kelas dan tahun ajaran.
6. Import dan export Excel untuk data master.
7. Absensi harian siswa.
8. Riwayat dan rekap absensi.
9. Pencatatan keterlambatan.
10. Pengelolaan pelanggaran kedisiplinan.
11. Sistem poin pelanggaran yang dapat dikonfigurasi.
12. Riwayat tindak lanjut kedisiplinan.
13. Laporan absensi dan kedisiplinan.
14. Audit perubahan data penting.

**Prinsip utama MVP:** bangun fitur inti yang benar-benar dapat digunakan, pastikan datanya akurat, dan pertahankan arsitektur yang memungkinkan ekspansi pada masa mendatang tanpa membangun ulang seluruh sistem.