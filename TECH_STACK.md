# TECH_STACK.md

# Tech Stack, Arsitektur, dan Standar Backend

> Dokumen ini menjadi pedoman teknis utama bagi developer maupun AI
> coding agent ketika membangun backend aplikasi absensi dan
> kedisiplinan sekolah.
>
> **Prinsip utama:** implementasi sederhana, aman, mudah diuji, mudah
> dipelihara, dan siap dikembangkan. Jangan menambahkan kompleksitas
> tanpa kebutuhan nyata.

## 1. Konteks dan Batasan Proyek

Aplikasi adalah sistem web untuk **satu sekolah pada tahap MVP**, dengan
kemungkinan pengembangan di masa depan. Backend menyediakan API untuk
frontend web.

Cakupan utama: - Autentikasi dan otorisasi berbasis peran (RBAC). -
Pengelolaan siswa, guru, kelas, tahun ajaran, dan penempatan siswa ke
kelas. - Absensi siswa oleh guru atau guru piket. - Pengelolaan kategori
pelanggaran dan catatan kedisiplinan siswa. - Rekap dan laporan absensi. -
Import/export Excel untuk data dan laporan. - Audit log untuk tindakan
penting.

Absensi guru dengan Geofencing, absensi mandiri siswa, absensi per mata
pelajaran, dan multi-tenant aktif **tidak termasuk MVP** sesuai `PRD.md`
§7. Bagian terkait di dokumen ini dipertahankan sebagai referensi masa
depan.

Di luar cakupan MVP: microservices, Kubernetes wajib, AI/computer
vision, face recognition, dan multi-tenant aktif. Struktur boleh dibuat
modular agar ekspansi kelak tidak membutuhkan penulisan ulang
besar-besaran.

## 2. Rekomendasi Tech Stack

### Backend inti

  -------------------------------------------------------------------------------
  Area                    Pilihan                         Penggunaan
  ----------------------- ------------------------------- -----------------------
  Bahasa                  Go (rilis stabil yang didukung; Bahasa backend
                          pin versi di                    
                          `go.mod`/toolchain)             

  HTTP framework          Gin                             Routing, middleware,
                                                          binding request

  Konfigurasi             Viper                           Memuat konfigurasi file
                                                          dan environment
                                                          variable

  Hot reload lokal        Air                             Memulai ulang server
                                                          saat kode berubah;
                                                          hanya untuk development

  Database utama          PostgreSQL                      Sumber data utama dan
                                                          transaksi

  Driver PostgreSQL       `pgx/v5`                        Koneksi dan operasi
                                                          PostgreSQL

  Query database          `sqlc`                          Menghasilkan kode Go
                                                          bertipe dari SQL yang
                                                          ditulis eksplisit

  Migrasi skema           `golang-migrate/migrate` atau   Migrasi database yang
                          `pressly/goose`                 berversi; pilih satu

  Cache/penyimpanan       Redis + `go-redis/v9`           Cache terukur, rate
  sementara                                               limit, atau data
                                                          sementara yang memang
                                                          diperlukan

  Queue (opsional)        RabbitMQ + `amqp091-go`         Pekerjaan asynchronous
                                                          dan worker

  Validasi                `go-playground/validator/v10`   Validasi bentuk dan
                                                          field request

  Logging                 `log/slog` dari standard        Structured logging,
                          library                         JSON di production

  Dokumentasi API         OpenAPI 3.0/3.1; `swaggo/swag`  Kontrak API dan
                          atau `oapi-codegen`             dokumentasi

  Excel                   `excelize`                      Membaca dan
                                                          menghasilkan file
                                                          `.xlsx`

  ID publik               UUID/ULID                       ID yang tidak
                                                          bergantung pada urutan
                                                          data yang mudah ditebak
  -------------------------------------------------------------------------------

**Pilihan awal yang direkomendasikan:** Gin + Viper + pgx + sqlc +
PostgreSQL. Gunakan SQL yang jelas dan terukur, bukan menambahkan ORM
secara otomatis. `sqlc` membantu menjaga query eksplisit dengan tipe
hasil yang dihasilkan saat build.

### Pengembangan, kualitas, dan operasi

  -----------------------------------------------------------------------
  Area                    Pilihan                 Kegunaan
  ----------------------- ----------------------- -----------------------
  Dependency              Go Modules              Versi dependency yang
                                                  dapat direproduksi

  Format                  `gofmt` dan `goimports` Format kode konsisten

  Linter                  `golangci-lint`         Memeriksa bug potensial
                                                  dan kualitas kode

  Unit/integration test   `testing` bawaan Go;    Pengujian otomatis
                          `testify` opsional      

  Test database           Testcontainers for Go   Integration test
                          atau PostgreSQL         realistis
                          sementara di CI         

  API testing             `httptest`,             Menguji handler dan
                          Bruno/Postman           kontrak API

  Local services          Docker Compose          Menjalankan PostgreSQL,
                                                  Redis, dan layanan
                                                  opsional

  CI                      GitHub Actions          Test, lint, build, dan
                                                  pemeriksaan migrasi

  Container               Docker multi-stage      Image production yang
                          build                   lebih kecil

  Metrics                 Prometheus              Metrik operasional

  Dashboard               Grafana                 Visualisasi metrik

  Tracing                 OpenTelemetry           Menelusuri request
                                                  lintas komponen jika
                                                  diperlukan

  Error tracking          Sentry atau layanan     Membantu investigasi
                          setara (opsional)       error production

  Secrets                 Environment/secret      Menyimpan kredensial di
                          manager platform        luar repository
                          deployment              
  -----------------------------------------------------------------------

Tidak semua komponen observability harus dipasang pada hari pertama.
Minimal mulai dengan structured logs, health checks, metrics dasar, dan
log error yang bisa ditelusuri.

## 3. Arsitektur yang Dipilih

Gunakan **Modular Monolith + Clean Architecture pragmatis**.

-   **Modular Monolith:** satu aplikasi API dan satu deployment utama,
    tetapi kode dipisahkan menjadi modul domain.
-   **Clean Architecture:** business rules tidak bergantung langsung
    pada Gin, PostgreSQL, Redis, atau RabbitMQ.
-   **Domain-oriented modules:** setiap fitur memiliki batas dan
    tanggung jawab yang jelas.
-   **Dependency direction:** handler memanggil use case; use case
    bergantung pada interface; implementasi repository/infrastruktur
    memenuhi interface tersebut.
-   **Hindari microservices pada MVP:** pemisahan proses deployment dan
    komunikasi jaringan menambah biaya operasional yang belum
    diperlukan.

### Diagram arsitektur

``` text
                 WEB FRONTEND
                      |
                 HTTPS / JSON
                      |
              [ Gin HTTP Router ]
                      |
        [ Middleware: Request ID, Logs,
          Recovery, Auth, RBAC, Rate Limit ]
                      |
                 [ Handlers ]
                      |
              [ Application / Use Cases ]
                      |
              [ Domain / Business Rules ]
                      |
             Interfaces (Repository/Ports)
                      |
         +------------+-------------+
         |            |             |
   [PostgreSQL]    [Redis]     [RabbitMQ]
   source of truth  optional     optional
         |                          |
   migrations                 [Background Worker]
                                    |
                          Excel / Notifications /
                          Other long-running jobs
```

Redis dan RabbitMQ bukan pengganti PostgreSQL. PostgreSQL tetap menjadi
sumber data utama. Tidak semua request harus melewati cache atau queue.

## 4. Pembagian Layer dan Aturan Tanggung Jawab

### 4.1 Transport / Handler Layer

Tanggung jawab: - Menerima HTTP request dan mengembalikan HTTP
response. - Binding dan validasi bentuk request. - Memanggil use case. -
Memetakan error aplikasi ke status HTTP. - Tidak berisi aturan bisnis
atau query SQL.

Handler tidak boleh mengakses database secara langsung.

### 4.2 Application / Use Case Layer

Tanggung jawab: - Menjalankan alur bisnis, misalnya
`RecordStudentAttendance`, `CheckInTeacher`, `ImportStudents`, atau
`CreateDisciplineRecord`. - Mengatur transaksi dan koordinasi
repository. - Memastikan aturan bisnis dijalankan sebelum data
disimpan. - Menggunakan interface untuk kebutuhan eksternal. - Menerima
`context.Context` untuk operasi yang dapat dibatalkan atau memiliki
deadline.

Aturan penting: use case tidak mengimpor Gin dan tidak bergantung pada
tipe request/response HTTP.

### 4.3 Domain Layer

Tanggung jawab: - Menyimpan model dan aturan bisnis inti. -
Mendefinisikan status, invariant, dan error domain. - Tidak bergantung
pada framework atau database.

Contoh aturan domain: - Status absensi hanya nilai yang diizinkan. -
Satu siswa tidak boleh memiliki dua catatan untuk kombinasi tanggal,
sesi, dan kelas yang sama. - Absensi mandiri hanya boleh dilakukan pada
periode yang dibuka sekolah. - Absensi guru diterima hanya jika validasi
lokasi, jadwal, dan aturan duplikasi lolos. - Perubahan data penting
dapat memerlukan alasan dan audit log.

Jangan membuat interface atau abstraksi jika belum ada kebutuhan nyata.

### 4.4 Repository / Persistence Layer

Tanggung jawab: - Menjalankan query PostgreSQL melalui `pgx` dan kode
hasil generate `sqlc`. - Menyediakan operasi penyimpanan untuk use
case. - Menangani transaksi database, constraint, dan pemetaan error
database. - Tidak mengandung aturan HTTP.

Query harus memakai parameter, bukan string concatenation dari input
pengguna. Gunakan constraint database untuk integritas yang penting,
bukan hanya validasi aplikasi.

### 4.5 Infrastructure Layer

Berisi implementasi teknis: - Koneksi dan pool PostgreSQL. - Redis
client. - RabbitMQ publisher/consumer bila queue diaktifkan. - Clock, ID
generator, file storage, dan integrasi eksternal. - Implementasi
logging, metrics, dan tracing.

### 4.6 Middleware Layer

Gunakan middleware untuk concern lintas fitur: - Request ID/correlation
ID. - Structured access logging. - Panic recovery. - Authentication. -
Authorization/RBAC pada route atau use case yang sesuai. - CORS dengan
allowlist. - Rate limiting. - Batas ukuran request dan timeout.

Jangan menganggap autentikasi saja cukup. Setiap endpoint harus
memeriksa hak akses dan batas data yang boleh diakses pengguna.

## 5. Struktur Folder yang Disarankan

``` text
school-attendance/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── worker/                 # hanya jika background worker diperlukan
│       └── main.go
├── internal/
│   ├── app/                    # bootstrap dan dependency wiring
│   ├── config/                 # konfigurasi typed dari Viper
│   ├── transport/http/
│   │   ├── router.go
│   │   ├── middleware/
│   │   └── response/
│   ├── modules/
│   │   ├── auth/
│   │   │   ├── domain/
│   │   │   ├── application/
│   │   │   ├── repository/
│   │   │   └── handler/
│   │   ├── students/
│   │   ├── teachers/
│   │   ├── classes/
│   │   ├── academic_years/
│   │   ├── student_attendance/
│   │   ├── teacher_attendance/
│   │   ├── discipline/
│   │   ├── reports/
│   │   └── imports/
│   ├── platform/
│   │   ├── database/
│   │   ├── cache/
│   │   ├── queue/
│   │   ├── logger/
│   │   ├── observability/
│   │   └── security/
│   └── shared/                 # hanya tipe/utilitas yang benar-benar umum
├── db/
│   ├── migrations/
│   └── query/                  # SQL source untuk sqlc
├── api/
│   └── openapi.yaml
├── configs/
│   └── config.example.yaml
├── deployments/
│   ├── Dockerfile
│   └── compose.yaml
├── scripts/
├── docs/
├── .github/workflows/
├── .env.example
├── .gitignore
├── .golangci.yml
├── Makefile
├── sqlc.yaml
├── go.mod
├── go.sum
└── README.md
```

Struktur ini adalah titik awal, bukan kewajiban untuk membuat semua
folder kosong sejak awal. Buat modul ketika fitur tersebut mulai
diimplementasikan. Hindari package `utils` atau `common` yang menjadi
tempat menumpuk kode tanpa batas yang jelas.

## 6. Pola Implementasi Wajib

### Dependency Injection

-   Buat dependency di composition root/bootstrap.
-   Kirim dependency melalui constructor, misalnya `NewService(...)`
    atau `NewHandler(...)`.
-   Hindari global mutable state, singleton tersembunyi, dan pemanggilan
    `viper.Get...` dari seluruh aplikasi.
-   Load konfigurasi sekali saat startup, validasi, lalu mapping ke
    struct konfigurasi bertipe.

### Error Handling

-   Gunakan error yang jelas dan dapat dibungkus dengan `%w`.
-   Pisahkan domain/application error dari detail error database.
-   Jangan mengirim stack trace, SQL, kredensial, atau detail internal
    kepada client.
-   Gunakan format response error konsisten, misalnya `code`, `message`,
    `request_id`, dan detail validasi yang aman.
-   Jangan mengabaikan error (`_ = err`) tanpa alasan yang
    terdokumentasi.

### Context, timeout, dan shutdown

-   Teruskan `context.Context` dari request ke use case dan query
    database.
-   Tetapkan timeout untuk koneksi, query, HTTP client, dan pekerjaan
    background.
-   Implementasikan graceful shutdown agar server berhenti menerima
    request baru dan menyelesaikan request aktif.
-   Jangan memakai `context.Background()` di dalam alur request jika
    `context` pemanggil tersedia.

### Database dan transaksi

-   Gunakan migration berversi; jangan mengubah skema production secara
    manual.
-   Pakai transaction untuk operasi yang harus berhasil atau gagal
    bersama.
-   Tambahkan foreign key, unique constraint, index, dan `NOT NULL`
    berdasarkan aturan data.
-   Gunakan pagination untuk daftar besar.
-   Hindari N+1 query; gunakan query yang dirancang untuk kebutuhan
    halaman.
-   Gunakan UTC untuk penyimpanan waktu dan tetapkan zona waktu sekolah
    untuk aturan jadwal/laporan.
-   Jangan menjalankan migrasi otomatis secara tak terkendali saat
    setiap replica API startup; jalankan sebagai langkah deployment yang
    terkontrol.

### Viper dan konfigurasi

-   Utamakan environment variable untuk konfigurasi deployment dan
    secret.
-   Simpan hanya contoh konfigurasi tanpa kredensial nyata di
    repository.
-   Validasi field wajib saat startup dan gagal lebih awal bila
    konfigurasi tidak valid.
-   Buat instance Viper yang terisolasi dan mapping ke struct; jangan
    mengandalkan global Viper dari banyak package.
-   Jangan log password, token, DSN lengkap, atau secret.

### Air

-   Air hanya untuk pengalaman development lokal.
-   Jangan memasukkan Air sebagai proses runtime production.
-   Pastikan folder cache/build output Air tidak menyebabkan rebuild
    berulang.

## 7. Keamanan

-   Gunakan HTTPS di deployment.
-   Hash password dengan algoritma password hashing yang sesuai, seperti
    Argon2id atau bcrypt; jangan simpan password asli.
-   Autentikasi memakai **JWT access token** berumur pendek di header
    `Authorization: Bearer` dan **refresh token opaque** yang dirotasi.
    Refresh token disimpan sebagai hash di database dan dikirim melalui
    cookie `HttpOnly`, `Secure`, `SameSite=Strict`. Access token tidak
    disimpan di `localStorage`.
-   Terapkan RBAC di server. Menyembunyikan tombol di frontend bukan
    kontrol akses.
-   Validasi semua input dan batasi ukuran request/upload.
-   Terapkan rate limit pada login dan endpoint yang rawan
    disalahgunakan.
-   Batasi CORS pada origin frontend yang diizinkan.
-   Gunakan query parameterized melalui `pgx`/`sqlc`.
-   Simpan secret melalui environment atau secret manager, bukan Git.
-   Catat tindakan sensitif dalam audit log tanpa merekam password,
    token, atau data sensitif berlebihan.
-   Tetapkan kebijakan backup, retensi, dan pemulihan database.

### Privasi dan Geofencing absensi guru (di luar MVP)

> Absensi guru dan Geofencing dikecualikan dari MVP oleh `PRD.md` §7
> butir 10 dan 16. Bagian ini hanya referensi jika fitur dimasukkan pada
> pengembangan berikutnya.

-   Minta izin lokasi browser ketika guru secara aktif melakukan
    check-in/check-out, bukan melacak lokasi terus-menerus di
    background.
-   Browser Geolocation API memerlukan secure context (umumnya HTTPS)
    dan izin pengguna.
-   Kirim latitude, longitude, dan accuracy ke backend; hitung jarak di
    server menggunakan rumus Haversine.
-   Backend membandingkan jarak dengan koordinat sekolah dan radius yang
    dikonfigurasi, lalu memvalidasi jadwal, status akun, dan duplikasi
    absensi.
-   Simpan hasil validasi seperlunya, termasuk waktu dan alasan
    penolakan yang aman.
-   Tangani lokasi tidak tersedia, izin ditolak, akurasi buruk, dan
    sinyal lemah.
-   Geofencing tidak membuktikan lokasi secara mutlak karena lokasi
    perangkat dapat dimanipulasi. Jangan menyebutnya sebagai mekanisme
    anti-kecurangan yang sempurna.

## 8. Strategi Redis dan RabbitMQ

### Redis --- opsional dan berdasarkan kebutuhan

Gunakan Redis jika ada manfaat yang terukur, misalnya: - Rate limiting
login/API. - Cache data referensi yang sering dibaca dan aman untuk
di-cache. - Penyimpanan sementara dengan TTL.

Jangan menjadikan Redis sumber kebenaran untuk catatan absensi. Pastikan
aplikasi tetap menangani cache miss dan invalidasi cache dengan benar.
Untuk MVP, backend dapat dimulai tanpa cache jika beban belum
membutuhkannya.

### RabbitMQ --- aktifkan saat pekerjaan asynchronous diperlukan

Contoh yang cocok: - Import Excel berukuran besar. - Pembuatan laporan
yang lama. - Pengiriman notifikasi yang tidak harus selesai dalam
request. - Pekerjaan terjadwal atau retryable.

Untuk absensi harian yang sederhana, simpan data langsung ke PostgreSQL
dalam transaksi. Jangan memasukkan setiap klik absensi ke queue tanpa
alasan kuat.

Jika RabbitMQ digunakan: - Consumer harus idempotent karena pesan dapat
dikirim ulang. - Gunakan acknowledgement setelah pekerjaan berhasil. -
Tetapkan retry policy, dead-letter queue, batas concurrency, dan
monitoring. - Jangan menganggap pengiriman pesan dan commit database
atomik secara otomatis. Untuk kebutuhan konsistensi yang penting,
pertimbangkan pola transactional outbox. - Pisahkan worker dari proses
API jika beban atau kebutuhan operasionalnya sudah membenarkan.

## 9. Standar API

-   Gunakan REST JSON dengan prefix versi, misalnya `/api/v1`.
-   Gunakan nama resource yang konsisten, misalnya `/students`,
    `/classes`, `/student-attendances`.
-   Terapkan status HTTP yang sesuai: `200`, `201`, `204`, `400`, `401`,
    `403`, `404`, `409`, `422`, dan `500` sesuai kondisi.
-   Gunakan pagination dan filter untuk endpoint daftar.
-   Validasi request dan dokumentasikan skema request/response di
    OpenAPI.
-   Gunakan idempotency atau unique constraint pada operasi yang
    berisiko menciptakan duplikasi, terutama absensi.
-   Jangan mengandalkan ID dari frontend sebagai bukti bahwa pengguna
    berhak mengakses suatu data.

Contoh endpoint awal: - `POST /api/v1/auth/login` - `GET /api/v1/me` -
`GET /api/v1/students` - `POST /api/v1/student-attendances` -
`GET /api/v1/student-attendances` -
`POST /api/v1/teacher-attendances/check-in` -
`POST /api/v1/teacher-attendances/check-out` -
`POST /api/v1/imports/students` - `GET /api/v1/reports/attendance`

Daftar tersebut adalah contoh desain, bukan kontrak final. Definisikan
request, response, izin akses, dan perilaku error sebelum
mengimplementasikan setiap endpoint.

## 10. Testing dan Quality Gate

Setiap fitur harus memiliki pengujian yang sepadan dengan risikonya.

-   **Unit test:** aturan domain dan use case, termasuk kasus berhasil
    dan gagal.
-   **Handler/API test:** status HTTP, validasi request, autentikasi,
    dan response error.
-   **Integration test:** query PostgreSQL, transaksi, constraint, dan
    migration.
-   **Security test:** RBAC, akses lintas kelas yang tidak diizinkan,
    input tidak valid, rate limit, dan kebocoran data.
-   **Attendance test:** duplikasi, status hadir/izin/sakit/alpa, dan
    batas jadwal.
-   **Geofencing test:** hanya jika fitur absensi guru dimasukkan ke
    scope pengembangan berikutnya.
-   **Excel test:** format salah, kolom wajib kosong, baris duplikat,
    file terlalu besar, dan laporan kosong.

Sebelum merge, minimal jalankan:

``` bash
gofmt -w .
go vet ./...
golangci-lint run
go test ./...
go build ./...
```

Tambahkan integration test dan pemeriksaan dependency/security dalam CI.
Jangan menganggap build berhasil sebagai bukti seluruh aturan bisnis
sudah benar.

## 11. Logging, Monitoring, dan Reliability

Minimal production: - Structured JSON logs dengan timestamp, level,
request ID, route, status, dan durasi. - Endpoint health check terpisah:
liveness dan readiness. - Metrics untuk request count, latency, error
rate, serta koneksi database. - Alert untuk error rate tinggi, database
tidak tersedia, dan antrean yang menumpuk jika RabbitMQ dipakai. -
Backup database terjadwal dan uji pemulihan secara berkala. - Graceful
shutdown dan timeout yang wajar.

OpenTelemetry, Prometheus, dan Grafana dapat ditambahkan secara
bertahap. Jangan mencatat informasi pribadi murid secara berlebihan ke
log atau metric label.

## 12. Development dan Deployment

### Lokal

Gunakan Docker Compose untuk PostgreSQL dan layanan pendukung yang
memang aktif. Air menjalankan API dengan hot reload. Sediakan
`.env.example`, seed data development, migration, dan perintah Makefile
yang konsisten.

Contoh target Makefile: - `make dev` - `make test` - `make lint` -
`make migrate-up` - `make sqlc` - `make build`

### Production

-   Build binary Go di multi-stage Dockerfile dan jalankan sebagai
    non-root.
-   Pisahkan konfigurasi dan secret dari image.
-   Gunakan TLS melalui platform deployment/reverse proxy.
-   Terapkan migration secara terkontrol sebelum atau selama release.
-   Gunakan backup, monitoring, dan prosedur rollback.
-   Untuk MVP, satu API instance dan satu PostgreSQL yang dikelola
    dengan baik dapat mencukupi; skala deployment berdasarkan metrik
    nyata.
-   Jangan mewajibkan Kubernetes sebelum kebutuhan availability dan
    skala mendukung kompleksitasnya.

## 13. Aturan untuk AI Coding Agent

AI coding agent wajib mengikuti aturan berikut:

1.  Baca dokumen ini dan PRD sebelum membuat atau mengubah kode.
2.  Implementasikan fitur berdasarkan use case dan domain, bukan menaruh
    seluruh logika di handler.
3.  Jangan menambahkan microservices, ORM, queue, cache, atau abstraksi
    baru tanpa alasan teknis yang spesifik.
4.  Gunakan dependency injection melalui constructor dan interface yang
    memang diperlukan.
5.  Jangan pernah menaruh secret atau kredensial nyata dalam kode, file
    contoh, log, atau commit.
6.  Semua endpoint harus memiliki validasi input, autentikasi/otorisasi
    yang sesuai, dan response error konsisten.
7.  Semua perubahan skema database harus berupa migration yang dapat
    ditinjau.
8.  Perubahan query harus disertai pengujian yang sesuai; utamakan
    constraint database untuk integritas kritis.
9.  Tambahkan unit test untuk aturan bisnis dan integration test untuk
    perilaku database penting.
10. Untuk import Excel, validasi file, batasi ukuran, berikan hasil
    validasi per baris, dan hindari memproses file besar secara sinkron
    tanpa batas. MVP memakai all-or-nothing. File disimpan di disk lokal
    terkonfigurasi (tidak publik); abstraksi object storage baru dibuat
    jika benar-benar dibutuhkan.
11. Jika absensi guru/Geofencing dimasukkan ke scope pengembangan
    berikutnya, perhitungan dan keputusan final dilakukan di backend;
    jangan mempercayai hasil keputusan dari frontend.
12. Jangan mengubah kontrak API atau struktur database secara diam-diam.
    Dokumentasikan perubahan dan dampaknya.
13. Jangan mengklaim pekerjaan selesai jika test, lint, atau build yang
    relevan belum dijalankan.
14. Pilih solusi Go yang idiomatis, eksplisit, mudah dibaca, dan mudah
    diuji dibanding abstraksi yang terlalu generik.
15. Jika requirement tidak jelas, catat asumsi dan pilih opsi paling
    sederhana yang tetap aman; jangan mengarang kebutuhan produk baru.

## 14. Urutan Implementasi yang Direkomendasikan

1.  Bootstrap aplikasi, konfigurasi, logging, Gin, health checks, Docker
    Compose, dan CI.
2.  PostgreSQL, migration, sqlc, dan pola repository/transaksi.
3.  Autentikasi, RBAC, serta audit log.
4.  Data master siswa, guru, kelas, dan tahun ajaran.
5.  Absensi siswa oleh guru/piket dan laporan dasar.
6.  Kedisiplinan siswa.
7.  Import/export Excel.
8.  Dashboard, laporan, dan hardening.
9.  Redis, RabbitMQ, dan observability lanjutan hanya jika ada kebutuhan
    yang jelas.

Absensi guru/Geofencing dan absensi mandiri siswa tidak termasuk urutan
MVP karena dikecualikan `PRD.md` §7.

## 15. Referensi Resmi

-   Go: https://go.dev/doc/
-   Gin: https://gin-gonic.com/docs/
-   Viper: https://github.com/spf13/viper
-   Air: https://github.com/air-verse/air
-   pgx: https://github.com/jackc/pgx
-   sqlc: https://docs.sqlc.dev/
-   PostgreSQL: https://www.postgresql.org/docs/
-   golang-migrate: https://github.com/golang-migrate/migrate
-   Redis Go client: https://github.com/redis/go-redis
-   RabbitMQ Go work queues:
    https://www.rabbitmq.com/tutorials/tutorial-two-go
-   Excelize: https://github.com/qax-os/excelize
-   OpenTelemetry Go: https://opentelemetry.io/docs/languages/go/
-   golangci-lint: https://golangci-lint.run/
-   Docker Compose: https://docs.docker.com/compose/
-   Go testing: https://go.dev/doc/tutorial/add-a-test

------------------------------------------------------------------------

## Keputusan Akhir

Untuk MVP, gunakan **Go + Gin + Viper + pgx + sqlc + PostgreSQL**,
dengan migration, structured logging, test, Docker Compose, dan CI
sebagai fondasi. Tambahkan **Redis** untuk kebutuhan cache/rate limiting
yang terukur. Tambahkan **RabbitMQ** hanya ketika pekerjaan asynchronous
seperti import Excel atau notifikasi memang membutuhkannya.

Arsitektur modular monolith dengan layer yang jelas memberi fondasi
profesional tanpa membuat proyek satu sekolah terlalu rumit.
Skalabilitas industri datang dari batas modul yang sehat, integritas
data, keamanan, observability, pengujian, dan proses deployment yang
konsisten---bukan dari banyaknya teknologi yang digunakan.
