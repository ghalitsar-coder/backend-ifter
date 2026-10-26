# backend-ifter

Backend API untuk aplikasi absensi dan kedisiplinan siswa (satu sekolah, MVP).

Dokumen sumber kebenaran:

- [`PRD.md`](PRD.md) — kebutuhan produk dan aturan bisnis.
- [`TECH_STACK.md`](TECH_STACK.md) — arsitektur dan standar engineering.
- [`DATABASE_DESIGN.md`](DATABASE_DESIGN.md) — desain database dan integritas data.

## Stack

Go, Gin, Viper, PostgreSQL, pgx/v5, sqlc, golang-migrate, `log/slog`.
Redis dan RabbitMQ belum dipakai; ditambahkan hanya jika ada kebutuhan terukur.

## Menjalankan lokal

Prasyarat: Go 1.26.10 (gunakan toolchain mise; `/usr/bin/go` di mesin ini
tidak kompatibel dengan stdlib-nya), Docker.

```bash
cp configs/config.example.yaml configs/config.yaml
cp .env.example .env
set -a; . ./.env; set +a
make tools
make db-up
make migrate-up
go run ./cmd/api
```

`make tools` memasang `migrate` dan `sqlc`. Pastikan `$(go env GOPATH)/bin`
ada di `PATH`. Konfigurasi dibaca dari `CONFIG_FILE` (default
`configs/config.yaml`) dan dapat dioverride dengan environment variable
berprefix `IFTER_`, misalnya `IFTER_DB_PASSWORD`.

`auth.jwt_secret` wajib diisi minimal 32 karakter. Nilai contoh hanya untuk
development; gunakan secret acak melalui `IFTER_AUTH_JWT_SECRET` di deployment.
`auth.allowed_origins` harus memuat origin frontend (contoh:
`http://localhost:3000`); request tanpa header `Origin` seperti curl tetap
diizinkan. Jika API berada di belakang reverse proxy, isi
`http.trusted_proxies` dengan IP/CIDR proxy agar rate limit dan access log
memakai IP klien yang benar.
Untuk membuat administrator pertama, isi environment berikut sekali saat
startup, lalu hapus password bootstrap dari environment:

```bash
export IFTER_AUTH_BOOTSTRAP_ADMIN_USERNAME=admin
export IFTER_AUTH_BOOTSTRAP_ADMIN_PASSWORD='ganti-password-ini'
export IFTER_AUTH_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
export IFTER_AUTH_BOOTSTRAP_ADMIN_FULL_NAME='Administrator Sekolah'
```

Bootstrap hanya berjalan jika belum ada akun berperan `admin`, sehingga aman
dijalankan berulang.

## Autentikasi

- `POST /api/v1/auth/login` — body `{"login":"username-atau-email","password":"..."}`.
- `POST /api/v1/auth/refresh` — rotasi refresh token dari cookie.
- `POST /api/v1/auth/logout` — mencabut refresh token dan menghapus cookie.
- `POST /api/v1/auth/activate` — aktivasi akun atau reset akses dengan
  `{"token":"...","password":"..."}`. Password minimal 12 karakter.
- `GET /api/v1/auth/me` — profil pengguna aktif dan daftar peran.

Endpoint pengelolaan akun, khusus peran `admin`:

- `GET /api/v1/users` — daftar akun (pagination `limit`/`offset`, filter `status`).
- `POST /api/v1/users` — buat akun `pending` dan kembalikan token invite sekali
  pakai beserta masa berlakunya.
- `GET /api/v1/users/:id` — detail akun dan peran.
- `PUT /api/v1/users/:id/roles` — ganti peran akun.
- `PATCH /api/v1/users/:id/status` — aktifkan/nonaktifkan akun.
- `POST /api/v1/users/:id/credential-reset` — terbitkan token reset sekali pakai.
- `GET /api/v1/roles` — daftar peran yang tersedia.

Access token JWT berumur pendek dikirim di `Authorization: Bearer`; refresh
token opaque disimpan sebagai hash di database dan dikirim lewat cookie
`HttpOnly`, `SameSite=Strict`, `Secure` di production. Endpoint login dan
aktivasi dibatasi rate limit per proses, dan endpoint yang memakai cookie
memeriksa `Origin`. Setiap token membawa `token_version` pengguna; reset atau
aktivasi password menaikkan versi itu sehingga access token lama langsung
ditolak tanpa menunggu kedaluwarsa.

Rotasi `auth.jwt_secret` tanpa memutus sesi: pindahkan secret lama ke
`auth.jwt_previous_secrets`, set secret baru di `auth.jwt_secret`, lalu deploy.
Token baru ditandatangani dengan secret baru, sedangkan token yang masih hidup
diverifikasi terhadap daftar secret lama. Hapus entri lama setelah melewati
`auth.access_token_ttl`.

Akun baru dibuat tanpa password default. Token invite/reset hanya dikembalikan
sekali kepada admin, disimpan di database sebagai hash, berlaku sekali, dan
tidak ditulis ke audit log. Masa berlaku diatur lewat
`auth.invite_token_ttl` (default 72 jam) dan `auth.reset_token_ttl` (default 1
jam). Akun `system` tidak dapat diubah, dan aksi yang akan menghapus
administrator aktif terakhir ditolak.

## Master data

Semua endpoint berikut khusus peran `admin` dan setiap perubahan menulis audit
log dalam transaksi yang sama.

Tahun ajaran:

- `GET /api/v1/academic-years` — daftar tahun ajaran.
- `POST /api/v1/academic-years` — body `name`, `start_date`, `end_date`, `is_active`.
- `GET /api/v1/academic-years/:id` — detail tahun ajaran.
- `PUT /api/v1/academic-years/:id` — ubah data atau aktifkan tahun ajaran.
  Hanya satu tahun ajaran yang aktif; rentang tanggal tidak boleh tumpang tindih.

Semester (`terms`):

- `GET /api/v1/academic-years/:id/terms` — daftar semester pada tahun ajaran.
- `POST /api/v1/academic-years/:id/terms` — body `term_number` (1–4), `name`,
  `start_date`, dan `end_date`.
- `GET /api/v1/terms/:id` — detail semester.
- `PUT /api/v1/terms/:id` — ubah nama, nomor, atau rentang tanggal.

Rentang semester harus berada di dalam tahun ajaran induknya dan tidak boleh
bertumpang tindih dengan semester lain pada tahun ajaran yang sama. Tabel
`terms` tidak memiliki kolom timestamp; response tidak memuat `created_at`
atau `updated_at`.

Kelas:

- `GET /api/v1/classes` — filter `academic_year_id`, `status`, pagination.
- `POST /api/v1/classes` — body `academic_year_id`, `name`, `grade_level`, `major`.
- `GET /api/v1/classes/:id` — detail termasuk wali kelas aktif.
- `PUT /api/v1/classes/:id` — ubah nama, tingkat, dan jurusan.
- `PATCH /api/v1/classes/:id/status` — `active` atau `archived`.
- `GET /api/v1/classes/:id/students` — daftar siswa aktif pada kelas saat ini.
- `PUT /api/v1/classes/:id/homeroom` — body `teacher_id`, opsional `start_date`;
  mengganti wali kelas aktif dan menyimpan riwayat penugasan.

Guru:

- `GET /api/v1/teachers` — pencarian `search` (nama/NIP), filter `status`, pagination.
- `POST /api/v1/teachers` — body `full_name` wajib; `employee_number`, `email`,
  `phone`, `user_id`, dan `status` opsional. `user_id` menautkan akun login.
- `GET /api/v1/teachers/:id` — detail guru.
- `PUT /api/v1/teachers/:id` — ubah data guru.
- `PATCH /api/v1/teachers/:id/status` — `active` atau `inactive`.

Siswa dan enrollment:

- `GET /api/v1/students` — pencarian `search` (nama/NIS), filter `class_id` dan
  `status`, pagination, pengurutan `sort=name|nis`.
- `POST /api/v1/students` — body `student_number`, `full_name`, `admission_year`
  wajib; field identitas lain opsional. `class_id` dan `class_start_date` dapat
  dipakai untuk langsung membuat enrollment.
- `GET /api/v1/students/:id` — detail identitas, kelas saat ini, dan riwayat kelas.
- `PUT /api/v1/students/:id` — ubah data identitas.
- `PATCH /api/v1/students/:id/status` — `active`, `graduated`, `transferred`, atau
  `inactive`. Status selain `active` menutup enrollment yang masih terbuka.
- `PUT /api/v1/students/:id/class` — body `class_id`, opsional `start_date`;
  memindahkan siswa dan menutup enrollment lama sehari sebelum tanggal mulai baru.

## Absensi

Absensi harian disimpan per siswa dan per tanggal. Hari efektif ditentukan oleh
rentang tahun ajaran kelas, hari operasional di `school_schedules`, dan kalender
libur `school_holidays`. Pengguna dengan peran `admin` dapat mengakses semua
kelas; `principal` hanya baca; `teacher` hanya kelas yang menjadi wali kelasnya
pada tanggal tersebut; `duty_teacher` hanya pada hari tugas piketnya.

Endpoint absensi untuk pengguna yang sudah login:

- `GET /api/v1/attendance/roster?class_id=...&date=YYYY-MM-DD` — daftar siswa,
  status tercatat, ringkasan belum diabsen, dan status penutupan hari.
- `POST /api/v1/attendance/bulk` — simpan banyak catatan sekaligus. Body
  `class_id`, `date`, dan `records` berisi `student_id`, `status`, opsional
  `check_in_at` (`HH:MM` atau RFC3339) dan `notes`. Siswa yang tidak dikirim
  tidak dihapus.
- `POST /api/v1/attendance/close` — tutup absensi satu kelas pada satu tanggal.
  Siswa yang belum dicatat diisi `absent` dengan sumber `system_close`.
- `GET /api/v1/attendance/records/:id` — detail catatan dan riwayat koreksi.
- `POST /api/v1/attendance/records/:id/corrections` — koreksi setelah hari
  ditutup; body `status`, `check_in_at`, `notes`, dan `reason` wajib. Koreksi
  lama, baru, alasan, pengguna, dan waktu disimpan.
- `GET /api/v1/attendance/history` — riwayat; filter `from`, `to`, `student_id`,
  `class_id`, `status`, pagination.
- `GET /api/v1/attendance/summary` — rekap per siswa/kelas; hanya hari yang sudah
  ditutup yang menjadi penyebut persentase `(hadir + terlambat) / hari efektif`.
- `GET /api/v1/exports/attendance` — unduh rekap absensi `.xlsx` dengan filter
  yang sama seperti summary.

Pembacaan pengaturan tersedia untuk pengguna login; perubahan khusus `admin`:

- `GET /api/v1/attendance/settings` dan `PUT /api/v1/attendance/settings` —
  nama sekolah, timezone, jam tutup otomatis, hari operasional, dan toleransi
  keterlambatan.
- `GET /api/v1/attendance/holidays`, `POST /api/v1/attendance/holidays`, dan
  `DELETE /api/v1/attendance/holidays/:date` — kalender libur.
- `GET /api/v1/duty-assignments`, `POST /api/v1/duty-assignments`, dan
  `PUT /api/v1/duty-assignments/:id/end` — penugasan guru piket per hari.

Setelah hari ditutup, catatan tidak boleh diubah langsung; perubahan harus
melalui koreksi. Penutupan otomatis berjalan setiap menit setelah
`attendance_auto_close_time` pada timezone sekolah untuk kelas aktif pada hari
efektif yang belum ditutup.

## Kedisiplinan

Kategori pelanggaran dikelola `admin`; kategori dinonaktifkan, bukan dihapus.
Poin bersifat opsional dan disalin dari `default_points` kategori saat catatan
dibuat. Kategori dengan `auto_on_attendance_status = 'late'` tidak dapat dicatat
manual karena berasal dari absensi.

Endpoint untuk pengguna login:

- `GET /api/v1/discipline/categories` — kategori aktif; `include_inactive=true`
  hanya untuk `admin`/`principal`.
- `POST /api/v1/discipline/categories`, `PUT /api/v1/discipline/categories/:id`,
  dan `PATCH /api/v1/discipline/categories/:id/status` — khusus `admin`.
- `GET /api/v1/discipline/records` — filter `from`, `to`, `student_id`,
  `class_id`, `category_id`, `recorded_by`, `status`, dan pagination.
- `POST /api/v1/discipline/records` — body `student_id`, `category_id`,
  `occurred_at`, dan `description`.
- `GET /api/v1/discipline/records/:id` — detail dan riwayat tindak lanjut.
- `POST /api/v1/discipline/records/:id/followups` — body `status` dan `notes`;
  status `cancelled` wajib disertai alasan.
- `PATCH /api/v1/discipline/records/:id/points` — khusus `admin`; body `points`
  dan `reason` wajib.
- `GET /api/v1/discipline/students/:id` — total poin, rekap per kategori, dan
  riwayat siswa.
- `GET /api/v1/exports/discipline` — unduh catatan kedisiplinan `.xlsx` dengan
  filter yang sama seperti daftar catatan.

Cakupan baca dan tulis mengikuti absensi: `admin` semua data, `principal` hanya
baca semua data, `teacher` hanya kelas yang menjadi wali kelasnya pada tanggal
kejadian, dan `duty_teacher` hanya pada hari piketnya. Status tindak lanjut:
`new`, `under_review`, `in_followup`, `completed`, `cancelled`.

Saat absensi berstatus `late`, sistem membuat catatan kedisiplinan otomatis
dalam transaksi yang sama jika ada kategori aktif dengan
`auto_on_attendance_status = 'late'`. Jika status absensi dikoreksi menjadi
selain `late`, catatan otomatis dibatalkan beserta alasan dan audit log; jika
dikembalikan ke `late`, catatan baru dibuat tanpa menggandakan catatan aktif.

## Dashboard, laporan, dan audit

Semua endpoint berikut memakai autentikasi dan cakupan data yang sama dengan
absensi/kedisiplinan: `admin` semua data, `principal` baca semua data,
`teacher` kelas yang menjadi wali kelas pada tanggal terkait, dan
`duty_teacher` hanya pada hari piketnya.

- `GET /api/v1/dashboard?date=YYYY-MM-DD&class_id=...` — ringkasan sesuai peran:
  jumlah siswa aktif/tercatat/belum tercatat, status kehadiran, catatan
  kedisiplinan bulan berjalan, tren 7 hari, daftar siswa terlambat, daftar
  siswa belum diabsen, dan aktivitas audit terbaru untuk `admin`. Jika `date`
  kosong, dipakai tanggal hari ini pada timezone sekolah.
- `GET /api/v1/reports/attendance` — laporan absensi terkelompok. Filter
  `from` dan `to` (atau `term_id`), `group_by=day|week|month`, `student_id`,
  dan `class_id`. Angka dihitung dari catatan absensi aktual pada rentang
  filter; baris memuat total tercatat per status dan persentase
  `(hadir + terlambat) / total tercatat`.
- `GET /api/v1/exports/attendance-report` — unduh laporan absensi terkelompok
  `.xlsx` dengan filter yang sama.
- `GET /api/v1/reports/discipline` — laporan kedisiplinan dengan filter
  `from`/`to` atau `term_id`, `student_id`, `class_id`, `category_id`,
  `recorded_by`, dan `status`. Response memuat total catatan, catatan
  dibatalkan, total poin catatan non-cancelled, rekap kategori/status, dan data.
- `GET /api/v1/audit-logs` — khusus `admin`; filter `action`, `entity_type`,
  `entity_id`, `actor_id`, `from`, `to`, serta `limit`/`offset`. `before_data`
  dan `after_data` dikembalikan sebagai JSON yang sudah di-whitelist saat
  penulisan audit. Filter tanggal memakai batas hari pada timezone sekolah dan
  diterjemahkan menjadi rentang `timestamptz` agar memakai index `created_at`.

## Hardening, performa, dan backup

Migration `000007_hardening` menambahkan index untuk filter status siswa/guru,
rentang waktu catatan kedisiplinan, filter aktor audit, dan riwayat enrollment
siswa. Query audit dan ringkasan kedisiplinan dashboard memakai batas
`timestamptz`, bukan fungsi tanggal per baris, sehingga PostgreSQL dapat
memakai index.

Test integrasi menyertakan smoke test performa: 200 siswa × 20 hari catatan
absensi, lalu dashboard dan laporan dijalankan dengan batas 5 detik per
request. Jalankan bersama test integrasi:

```bash
make db-up
make test-integration
```

CI menjalankan format, vet, test dengan race detector dan profil coverage,
integration test, build, `go mod verify`, `govulncheck`, dan lint. Profil
coverage diunggah sebagai artifact; Dependabot memeriksa update modul Go dan
GitHub Actions setiap minggu. `GET /metrics` menyediakan metrik Prometheus untuk
request dan pool database; endpoint ini hanya didaftarkan bila
`hardening.metrics_token` diisi dan wajib diakses dengan
`Authorization: Bearer <token>`.

Koneksi database didaur ulang setelah `db.max_conn_lifetime`, koneksi idle
ditutup setelah `db.max_conn_idle_time`, dan setiap statement dibatasi
`db.statement_timeout` (default 15 detik). Setiap request menerima konteks
dengan batas `hardening.request_timeout` (default 30 detik); handler yang
melewati batas sebelum menulis respons dikembalikan sebagai 504. Ingest tap
RFID me-resolve seluruh kartu dalam satu query per batch, bukan satu query per
tap.

Rate limit login dan ingest disimpan di memori proses. Bila API dijalankan
lebih dari satu replika tanpa sticky session, pasang rate limit tambahan di
reverse proxy atau pindahkan limiter ke Redis.

Verifikasi ala ECC dijalankan per fase dengan `make verify-phase`:

| Phase | Cakupan |
| --- | --- |
| `1` / `foundation` | auth, config, RBAC/transport, migration |
| `2` / `master-data` | tahun ajaran, kelas, guru, siswa, enrollment |
| `3` / `imports` | template, import, export Excel |
| `4` / `attendance` | absensi, penutupan, koreksi, rekap |
| `5` / `discipline` | kategori, catatan, poin, tindak lanjut |
| `6` / `reporting` | dashboard, laporan, audit, metrik |

Setiap gate menjalankan `gofmt`, build, vet, race test dengan coverage, dan
lint untuk paket fase tersebut. Jalankan `make security` untuk pemeriksaan
dependency. Test integrasi memerlukan PostgreSQL aktif.

Backup dan pemulihan memakai `pg_dump`/`pg_restore` di container PostgreSQL,
bukan instalasi host:

```bash
make backup
# backups/ifter-YYYYmmdd-HHMMSS.dump

make restore file=backups/ifter-YYYYmmdd-HHMMSS.dump
```

Restore bersifat destruktif (`--clean --if-exists`) pada database target.
Hentikan API yang memakai database tersebut terlebih dahulu. Simpan backup di
luar host/volume Docker, enkripsi saat disimpan, dan uji restore ke database
scratch secara berkala. Kebijakan retensi minimum yang disarankan: 7 backup
harian dan 4 backup mingguan; sesuaikan dengan kebijakan sekolah.

Job maintenance berjalan sekali saat start lalu setiap
`maintenance.interval` (default 24 jam):

- `maintenance.refresh_token_retention` (default `720h`) menghapus token
  refresh yang sudah kedaluwarsa lebih lama dari nilai ini.
- `maintenance.audit_log_retention` (default `0`) mematikan pemangkasan audit
  log. Bila diisi, mis. `8760h`, audit log yang lebih tua dihapus permanen.
  Penghapusan hanya berjalan di dalam transaksi job yang menyalakan flag lokal;
  `DELETE`/`UPDATE`/`TRUNCATE` manual tetap ditolak trigger append-only.
  Aktifkan hanya bila kebijakan retensi sekolah mengizinkannya.

### Checklist produksi

- Terminasi TLS di reverse proxy/load balancer. Proses API sendiri tidak
  menangani sertifikat; header HSTS hanya dikirim bila request tiba lewat TLS,
  jadi pasang HSTS di proxy. Jangan meneruskan HTTP polos ke publik.
- Isi `http.trusted_proxies` hanya dengan alamat proxy agar rate limit membaca
  IP klien sebenarnya, dan batasi endpoint `/metrics` di jaringan internal.
- Set `app.env=production`, `auth.cookie_secure=true`,
  `db.sslmode=require`/`verify-full`, serta token metrics acak. Konfigurasi
  menolak start bila `sslmode=disable` atau cookie tidak secure di produksi.
- Backup harian saja memberi RPO hingga 24 jam. Untuk RPO lebih kecil, aktifkan
  arsip WAL berkelanjutan (mis. pgBackRest) dan uji restore ke database scratch
  minimal sekali per kuartal.
- Kirim log JSON dan metrik ke penyimpanan terpusat; audit log tidak ikut
  dihapus oleh job kecuali retensi audit diaktifkan.

## Excel import & export

Semua endpoint berikut khusus peran `admin`. File disimpan privat di
`imports.dir` dengan mode `0600`; direktori tersebut tidak boleh disajikan
sebagai static file. Di container, mount direktori itu sebagai volume persisten
agar file job yang belum dikonfirmasi tidak hilang saat restart.

Template dan export:

- `GET /api/v1/import-templates/students` — unduh template import siswa (`.xlsx`).
- `GET /api/v1/import-templates/teachers` — unduh template import guru (`.xlsx`).
- `GET /api/v1/exports/students` — export siswa; filter `status`, `class_id`,
  dan `search`.
- `GET /api/v1/exports/teachers` — export guru; filter `status` dan `search`.

Export siswa memakai header yang sama dengan template import, termasuk
`Tahun Ajaran`, `Kelas`, dan `Tanggal Mulai Kelas`, sehingga hasil export dapat
diunggah kembali dengan mode `upsert`. Export guru juga memakai header template
guru.

Template berisi sheet data utama, sheet `Contoh` berisi satu baris contoh yang
valid, dan sheet `Petunjuk` berisi aturan pengisian.

Import dua tahap:

- `POST /api/v1/imports/students` — multipart `file` (`.xlsx`) dan `mode`
  (`create_only` atau `upsert`, default `create_only`). Server memvalidasi seluruh
  baris dan mengembalikan ringkasan, preview per baris, serta error per baris.
- `POST /api/v1/imports/teachers` — sama untuk data guru.
- `GET /api/v1/imports` — riwayat job; filter `entity_type`, `status`, pagination.
- `GET /api/v1/imports/:id` — detail job, error per baris, dan preview untuk job
  yang masih `validated`.
- `POST /api/v1/imports/:id/confirm` — terapkan seluruh baris valid dalam satu
  transaksi.

Aturan import:

- Identitas siswa adalah NIS; identitas guru adalah NIP.
- `create_only` menolak NIS/NIP yang sudah ada. Mode `upsert` memperbarui baris
  yang ada dan tidak menimpa nilai lama dengan sel kosong.
- Duplikat di dalam file, referensi kelas yang tidak ditemukan, dan data tidak
  valid membuat job berstatus `invalid`. Job hanya dapat dikonfirmasi jika
  berstatus `validated`; import bersifat all-or-nothing.
- Kelas mengacu pada nama kelas dan nama tahun ajaran. Tahun ajaran kosong
  berarti tahun ajaran aktif. Tanggal mulai kelas harus berada di dalam rentang
  tahun ajaran.
- Siswa yang sudah punya enrollment terbuka di kelas lain ditolak; pindah kelas
  tetap dilakukan melalui endpoint master data.
- Saat konfirmasi, file dibaca ulang, checksum diverifikasi, dan data divalidasi
  ulang. Jika ringkasan berubah sejak preview, job menjadi `invalid` dan file
  harus diunggah ulang.
- Import guru tidak membuat akun login, password default, atau peran pengguna.
- Batas default: 5 MiB per file dan 2000 baris. File asli dihapus setelah 7 hari;
  error per baris disimpan 30 hari. Batas dapat diubah lewat konfigurasi
  `imports`.

Health check:

- `GET /healthz` — liveness, tidak menyentuh database.
- `GET /readyz` — readiness, ping database.
- `GET /metrics` — metrik Prometheus: jumlah dan latensi request per route/status
  serta koneksi pool database. Endpoint hanya didaftarkan bila
  `hardening.metrics_token` (env `IFTER_HARDENING_METRICS_TOKEN`) diisi, dan
  akses wajib menyertakan `Authorization: Bearer <token>`. Jika token kosong,
  scrape metrics harus lewat jalur lain (mis. sidecar) atau endpoint tidak
  tersedia sama sekali.

`POST /api/v1/device-taps` dibatasi per IP klien (`hardening.ingest_rate_limit`,
default 600 request/menit). Di balik NAT sekolah, naikkan batas atau pastikan
`http.trusted_proxies` benar agar `ClientIP()` memakai alamat asli.

## End-to-end (e2e)

Test e2e memakai `tester-army/e2e` (`e2e/`) dan menjalankan API sungguhan di
atas database terpisah. Suite ini tidak mengubah atau menggantikan test Go;
test Go tetap menjadi tempat utama untuk cabang logika dan edge case,
sedangkan e2e memverifikasi alur lewat HTTP.

```bash
cd e2e && npm install      # sekali saja
cd .. && make e2e
```

`make e2e` menjalankan `scripts/e2e.sh` yang:

- memakai database `ifter_e2e` dan port `8081` (dapat dioverride lewat
  `E2E_DB` dan `E2E_PORT`),
- menghapus lalu membuat ulang database e2e sebelum setiap run,
- menerapkan migration, menjalankan API dengan bootstrap admin `e2e-admin`,
  menunggu `/readyz`, menjalankan suite, lalu mematikan API,
- menyimpan log API ke `/tmp/opencode/ifter-e2e-server.log` jika ada
  kegagalan.

Suite e2e bersifat deterministik, memanggil API dengan `fetch`, dan tidak
memerlukan AI/model key. File test ada di `e2e/tests/api.e2e.ts`; config dan
target ada di `e2e/e2e.config.ts`. Untuk menjalankan suite terhadap API yang
sudah hidup:

```bash
cd e2e
API_URL=http://127.0.0.1:8082 npx e2e run
```

Jangan arahkan e2e ke database development atau produksi: suite ini membuat,
mengubah, dan menghapus data.

## Perintah

```bash
make run              # jalankan API
make dev              # hot reload dengan Air
make test             # unit test (integration test di-skip tanpa TEST_DATABASE_URL)
make test-integration # auth + database integration test; butuh PostgreSQL
make e2e              # API e2e di database ifter_e2e + port 8081
make vet              # go vet
make lint             # golangci-lint (v2)
make build            # build ke bin/api
make db-up            # PostgreSQL via Docker Compose
make db-down          # hentikan Compose
make migrate-up       # terapkan semua migration
make migrate-down     # rollback satu migration
make migrate-create name=add_x
make sqlc             # regenerasi internal/db
make backup           # dump database ke backups/ (Docker Compose)
make restore file=backups/ifter-YYYYmmdd-HHMMSS.dump
make verify-phase PHASE=4   # gate verifikasi per fase (1-6)
make security         # go mod verify + govulncheck
```

Test integrasi memakai database sementara yang dibuat dan dihapus otomatis.
Jalankan dengan PostgreSQL lokal:

```bash
make db-up
make test-integration
```

## Struktur

```text
cmd/api/                     entrypoint
internal/app/                composition root
internal/auth/               login, refresh rotation, akun, RBAC identity, audit
internal/masterdata/         tahun ajaran, kelas, guru, siswa, enrollment, wali kelas
internal/imports/            template, validasi, preview, confirm, export Excel
internal/attendance/         absensi harian, penutupan, koreksi, rekap, export
internal/discipline/         kategori, catatan pelanggaran, poin, tindak lanjut
internal/reporting/          dashboard, audit log, dan laporan lintas modul
internal/config/             konfigurasi Viper bertipe + validasi
internal/platform/           database (pool, transaksi, pemetaan error), logger
internal/transport/http/     router, middleware, response
internal/db/                 kode sqlc (generated)
db/migrations/               migration SQL (golang-migrate)
db/query/                    query sumber sqlc
configs/                     contoh konfigurasi
deployments/                 Dockerfile
```

## Status

Fase 1 (bootstrap), Fase 2 (skema awal + sqlc + transaksi), Fase 3
(autentikasi JWT + refresh rotation, RBAC, audit), Fase 4 (pengelolaan akun,
invite/reset akses), Fase 5 (master data: tahun ajaran, kelas, guru, siswa,
enrollment, wali kelas), dan Fase 6 (template, import, validasi/preview,
confirm, export Excel), Fase 7 (absensi harian, hari efektif, penutupan
manual/otomatis, koreksi, rekap, export, pengaturan sekolah, libur, dan
penugasan guru piket), Fase 8 (kategori kedisiplinan, catatan pelanggaran,
poin, tindak lanjut, integrasi keterlambatan otomatis, rekap siswa, dan export),
dan Fase 9 (semester, dashboard per peran, laporan absensi terkelompok beserta
export, laporan kedisiplinan, dan audit log) selesai. Sprint 6 hardening
(index performa, smoke test performa, metrik Prometheus, serta perintah
backup/restore) juga selesai; prosedur operasionalnya ada di bagian Hardening,
performa, dan backup. Suite e2e API ada di `e2e/` dan dijalankan dengan
`make e2e`. CI menjalankan integration test dan pemindaian kerentanan
dependency.
