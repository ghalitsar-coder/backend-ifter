# Fitur Frontend yang Perlu Menyusul Backend

Dokumen handoff untuk agen/AI yang mengerjakan repo frontend
(`frontend-ifter`). Window backend hanya membangun API; semua pekerjaan UI
dicatat di sini agar bisa dikerjakan paralel tanpa menebak kontrak.

Status terakhir: 2026-10-26.

## Aturan umum

- Base URL: `/api/v1`. Autentikasi memakai bearer access token yang sama
  dengan endpoint lain.
- Semua respons sukses memakai envelope yang sudah dipakai frontend:
  `{ "data": [...] }` untuk list, objek bernama untuk detail
  (`{ "device": {...} }`, `{ "subscription": {...} }`).
- Error memakai bentuk datar `{ "code": "...", "message": "...", "request_id": "..." }`
  dari `response.Error` (`request_id` opsional).
- Gating role di frontend bersifat kosmetik; server tetap otoritatif.
- Label domain ditulis hard-coded bahasa Indonesia (tidak menambah key locale
  baru), kecuali disebut lain.

Kontrak akibat hardening backend (P1–P3, tidak butuh fitur baru, cukup pastikan
error handler umum menangani ini):

- Setiap request dibatasi 30 detik (`hardening.request_timeout`); kalau lewat,
  server menjawab 504 `request_timeout`. Untuk GET, aman ditampilkan sebagai
  "muat ulang / coba lagi"; jangan retry otomatis untuk mutation.
- Rate limit in-process menghasilkan 429 `rate_limited` dengan header
  `Retry-After` (detik): login/activate 30/menit per IP, ingest device
  600/menit per IP. Tampilkan pesan "terlalu banyak percobaan, tunggu
  N detik" bila ini terjadi.
- Access token bisa menjadi tidak sah seketika saat password direset
  (`token_version`), jadi 401 `unauthorized` bukan hanya berarti token
  kedaluwarsa. Alur frontend tetap: 401 → coba refresh → refresh gagal →
  clear sesi dan arahkan ke login.

---

## F6 — Label export presensi (frontend-only, sudah setengah jadi)

**Status backend:** tidak perlu backend.

Di working tree frontend sudah ada perubahan `AttendanceManager.tsx` +
`AttendanceToolbar.tsx`: `onDownloadPdf` → `onDownloadExcel`, label
"Unduh Excel (XLSX)". Belum di-commit karena dua key locale menjadi tidak
terpakai: `AttendancePage.download_pdf` dan `AttendancePage.toast_pdf_generating`.

**Tugas frontend:**

1. Hapus dua key tersebut dari `src/locales/en.json` dan `fr.json`
   (editan locale lain milik user harus dipertahankan).
2. Jalankan gate frontend (`check:types`, `test`, `lint`, `check:deps`,
   `check:i18n`), lalu commit bersama kedua file presensi di atas.

**Kriteria terima:** tombol export berlabel Excel/XLSX; `check:i18n` exit 0
tanpa warning; tidak ada key locale baru.

---

## F9 — Notifikasi web-push

**Status backend:** diimplementasikan di wave ini (migration `000018_push_subscriptions`).

### Kontrak backend

Konfigurasi server (bukan tanggung jawab frontend):

| Env | Arti |
|-----|------|
| `IFTER_WEBPUSH_ENABLED` | `true` untuk mengaktifkan pengiriman |
| `IFTER_WEBPUSH_PUBLIC_KEY` | VAPID public key (base64url) |
| `IFTER_WEBPUSH_PRIVATE_KEY` | VAPID private key (base64url) |
| `IFTER_WEBPUSH_SUBJECT` | kontak VAPID, mis. `mailto:admin@sekolah.sch.id` |

Endpoint (semua butuh login):

| Method | Path | Role | Keterangan |
|--------|------|------|------------|
| GET | `/push/config` | semua user login | `{ "enabled": bool, "public_key": string }`; `public_key` kosong bila nonaktif |
| GET | `/push/subscriptions` | semua user login | `{ "data": [...] }` milik pemanggil; field: `id`, `endpoint`, `user_agent`, `created_at`, `updated_at` (p256dh/auth tidak pernah dikembalikan) |
| POST | `/push/subscriptions` | semua user login | body `{ endpoint, p256dh, auth, user_agent? }` → 201 `{ "subscription": {...} }`; upsert berdasarkan `endpoint` (endpoint yang sama berpindah ke user pemanggil) |
| DELETE | `/push/subscriptions/:id` | semua user login | hanya milik sendiri; 204; 404 bila bukan miliknya |
| POST | `/admin/push/test` | admin | body `{ title, body }` → `{ "sent": n, "failed": n }`; mengirim ke subscription milik admin pemanggil |

Bila `WEBPUSH_ENABLED=false`: GET `/push/config` tetap 200 (`enabled:false`),
POST subscription dan test-send mengembalikan 503 `push_disabled`.

Notifikasi otomatis yang sudah terpasang di backend: saat pelanggaran
kedisiplinan dibuat, push dikirim ke subscription milik user ber-role
`admin` dan `principal`. Judul/body sudah diformat server; frontend hanya
menampilkan payload.

Format payload push (JSON):

```json
{
  "title": "Pelanggaran baru",
  "body": "<nama siswa> — <kategori> (<poin> poin)",
  "url": "/dashboard/kedisiplinan"
}
```

### Tugas frontend

1. Service worker (`public/sw.js` atau setara) yang menangani event `push`
   dan `notificationclick` (fokus/ buka `url`).
2. Registrasi service worker + minta izin `Notification` dari halaman
   Pengaturan (tab Notifikasi) atau banner.
3. Ambil public key dari `GET /push/config`; bila `enabled=false`, tampilkan
   state "Notifikasi belum diaktifkan server" dan jangan minta izin.
4. `pushManager.subscribe({ userVisibleOnly: true, applicationServerKey })`,
   lalu kirim `endpoint`, `p256dh`, `auth`, `user_agent` ke
   `POST /push/subscriptions`.
5. Daftar perangkat notifikasi milik user dengan tombol "Putuskan"
   (`DELETE /push/subscriptions/:id`); unsubscribe juga memanggil
   `pushManager.unsubscribe()`.
6. Tombol "Kirim notifikasi uji" khusus admin (`POST /admin/push/test`).
7. Tangani 503 `push_disabled` dengan pesan yang jelas, bukan toast error
   generik.

### Kriteria terima

- Izin hanya diminta setelah config menyatakan `enabled`.
- Subscription bertahan setelah reload; token kedaluwarsa/`410 Gone` dari
  push service ditangani dengan menghapus subscription lokal.
- Tidak ada key locale baru.

---

## F10 — Registry perangkat (RFID/gate)

**Status backend:** diimplementasikan di wave ini (migration `000017_devices`).

### Kontrak backend

Admin-only (`role = admin`).

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/devices?status=active\|inactive&type=rfid_reader\|gate\|tablet\|other` | `{ "data": [...] }` |
| POST | `/devices` | body `{ name, identifier, device_type, location?, notes? }` → 201 `{ "device": {...}, "ingest_token": "dt_..." }` |
| GET | `/devices/:id` | `{ "device": {...} }` |
| PUT | `/devices/:id` | body `{ name, identifier, device_type, location?, notes? }` → 200 `{ "device": {...} }` |
| PATCH | `/devices/:id/status` | body `{ status: "active"\|"inactive" }` → 200 `{ "device": {...} }` |

Objek `device`:

```json
{
  "id": "uuid",
  "name": "Reader Gerbang Utama",
  "identifier": "RFID-GATE-01",
  "device_type": "rfid_reader",
  "location": "Gerbang depan",
  "notes": null,
  "status": "active",
  "last_seen_at": null,
  "created_at": "...",
  "updated_at": "..."
}
```

Aturan: `identifier` unik (409 `conflict` bila duplikat); perangkat tidak
dihapus, cukup dinonaktifkan. `last_seen_at` diisi oleh ingest tap RFID (F11).
`ingest_token` hanya muncul sekali di respons pembuatan — jangan disimpan di
state jangka panjang; lihat F11 untuk rotasi.

### Tugas frontend

1. Halaman admin `/dashboard/perangkat` + entri nav.
2. Tabel dengan filter status/tipe, badge status, kolom `last_seen_at`
   ("Belum pernah" bila null).
3. Modal tambah/ubah: nama, identifier, tipe, lokasi, catatan; validasi
   identifier unik ditampilkan dari error 409.
4. Aksi aktif/nonaktifkan dengan konfirmasi (`PATCH .../status`).
5. Empty/error state Indonesia; tombol tulis hanya untuk admin.

### Kriteria terima

- Identifier duplikat menampilkan pesan spesifik, bukan error generik.
- Nonaktifkan langsung terlihat di tabel tanpa reload.
- Tidak ada key locale baru.

---

## F7 — Portal orang tua

**Status backend:** diimplementasikan di wave ini (migration `000019_parent_portal`).

### Kontrak backend

Role baru `parent` sudah ada di tabel `roles` dan bisa diberikan lewat
`PUT /users/:id/roles` yang sudah ada. Tautan akun ortu → siswa disimpan di
`parent_students` (banyak anak per akun, satu anak boleh punya beberapa wali).

Admin-only (role `admin`):

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/users/:id/children` | `{ "data": [...] }` tautan milik user itu |
| PUT | `/users/:id/children` | body `{ "children": [{ "student_id": "uuid", "relationship": "father"\|"mother"\|"guardian" }] }` (relationship opsional) → 200 `{ "data": [...] }` |

`PUT` bersifat **replace-all**: seluruh tautan lama dihapus lalu diganti isi
payload. Kirim `{"children": []}` untuk mengosongkan. Aturan:

- Target user harus sudah ber-role `parent` → jika belum: 409 `conflict`
  (`assign the parent role to this user before linking children`).
- User atau siswa tidak dikenal → 404; UUID rusak, relationship di luar enum,
  siswa duplikat dalam satu payload, atau lebih dari 20 anak → 400.
- Siswa berstatus `graduated` boleh tetap tertaut (riwayat tetap terbaca).
- Setiap perubahan dicatat di audit log `user.children.update`.

Parent-only (role `parent`, akun yang tidak punya role ini → 403):

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/parent/children` | `{ "data": [...] }` anak yang tertaut ke pemanggil |
| GET | `/parent/children/:id/attendance?from=&to=` | sama dengan `/my/attendance` |
| GET | `/parent/children/:id/attendance-summary?from=&to=` | sama dengan `/my/attendance-summary` |
| GET | `/parent/children/:id/discipline?from=&to=` | sama dengan `/my/discipline` |
| GET | `/parent/children/:id/discipline-summary?from=&to=` | sama dengan `/my/discipline-summary` |
| GET | `/parent/children/:id/dispensations?limit=&offset=` | `{ "data": [...], "total", "limit", "offset" }` |

`from`/`to` wajib (`YYYY-MM-DD`, rentang maksimal 366 hari), persis seperti
`/my/*`. Anak yang tidak tertaut ke pemanggil **selalu 404** (bukan 403) agar
keberadaan siswa lain tidak bisa ditebak; UUID rusak → 400.

Objek `parent child`:

```json
{
  "student_id": "uuid",
  "student_number": "SAC-001",
  "full_name": "Ani Berlink",
  "status": "active",
  "class_id": "uuid",
  "class_name": "12A",
  "relationship": "mother",
  "linked_at": "2026-10-25T09:00:00Z"
}
```

`class_id`/`class_name`/`relationship` bernilai `null` bila tidak ada
(kelas mengikuti enrollment yang aktif hari ini; relationship opsional).

### Tugas frontend

1. Layout/role `parent` di navigasi, terpisah dari `student`.
2. Halaman admin: pilih user ber-role `parent`, kelola banyak anak sekaligus
   (multi-select + relationship) memakai `GET`/`PUT /users/:id/children`.
3. Portal ortu: pemilih anak (dari `/parent/children`, langsung tampil bila
   hanya satu), lalu tab Kehadiran / Kedisiplinan / Dispensasi yang memakai
   endpoint `/parent/children/:id/*`.
4. Filter tanggal yang sama dengan portal siswa; tampilkan nama anak dan kelas
   dari objek child, bukan query tambahan.
5. Tangani 409 `conflict` saat menautkan user yang belum ber-role `parent`
   dengan pesan spesifik; 404 untuk anak tak tertaut sebagai empty state
   "Anak tidak terhubung ke akun ini".

### Kriteria terima

- Akun tanpa role `parent` tidak melihat menu portal ortu dan mendapat pesan
  jelas bila admin mencoba menautkan.
- Satu akun ortu bisa berpindah anak tanpa reload penuh.
- Tidak ada key locale baru.

---

## F8 — Absensi per mata pelajaran

**Status backend:** diimplementasikan di wave ini (migration `000020_subject_attendance`).
Absensi harian (`/attendance/*`) tidak berubah dan tetap menjadi sumber utama
kehadiran sekolah; F8 menambah presensi per pertemuan pelajaran.

### Kontrak backend

Katalog mapel dan penugasan mengajar (master data):

| Method | Path | Role | Keterangan |
|--------|------|------|------------|
| GET | `/subjects?search=&is_active=` | semua staff | `{ "data": [...] }` |
| GET | `/subjects/:id` | semua staff | `{ "subject": {...} }` |
| POST | `/subjects` | admin + staff_tu | body `{ code, name, description?, is_active? }` → 201 |
| PUT | `/subjects/:id` | admin + staff_tu | `is_active` yang dihilangkan berarti nilai lama dipertahankan |
| GET | `/teaching-assignments?teacher_id=&subject_id=&class_id=&is_active=` | semua staff | `{ "data": [...] }`; guru biasa hanya melihat penugasan miliknya |
| GET | `/teaching-assignments/:id` | semua staff | `{ "assignment": {...} }`; penugasan guru lain → 403 |
| POST | `/teaching-assignments` | admin + staff_tu | body `{ teacher_id, subject_id, class_id, is_active? }` → 201 |
| PUT | `/teaching-assignments/:id` | admin + staff_tu | triple duplikat → 409 |

Sesi pelajaran dan presensi:

| Method | Path | Role | Keterangan |
|--------|------|------|------------|
| POST | `/attendance-sessions` | admin + teacher + duty_teacher | body `{ teaching_assignment_id, session_date, period_start, period_end, topic?, notes? }` → 201 `{ "session": {...} }` |
| GET | `/attendance-sessions?from=&to=&class_id=&subject_id=&teacher_id=&status=&limit=&offset=` | admin + teacher + duty_teacher + principal | `{ "data": [...], "total", "limit", "offset" }` |
| GET | `/attendance-sessions/:id` | sama | `{ "session": {...} }` |
| GET | `/attendance-sessions/:id/roster` | sama | `{ "data": [roster row] }` siswa terdaftar di kelas saat tanggal sesi |
| GET | `/attendance-sessions/:id/records` | sama | `{ "data": [record] }` record yang sudah tersimpan |
| PUT | `/attendance-sessions/:id/records` | admin + teacher + duty_teacher | body `{ "records": [{ student_id, status, note? }] }` → `{ "data": [...] }` (upsert) |
| POST | `/attendance-sessions/:id/close` | admin + teacher + duty_teacher | → 200 `{ "session": {...} }`; sesi tertutup permanen |
| GET | `/reports/subject-attendance?from=&to=&class_id=&subject_id=&teacher_id=` | semua staff | `{ "from", "to", "data": [...], "totals": {...} }` |

Aturan:

- `from`/`to` wajib untuk list sesi dan rekap (`YYYY-MM-DD`, maksimal 366
  hari) → 400 bila hilang/rusak.
- Guru biasa (role `teacher` tanpa role lain) hanya bisa membuat/membaca sesi
  dan penugasan miliknya; `admin` dan `duty_teacher` melihat seluruh sekolah;
  `principal`/`staff_tu` hanya baca dan melihat seluruh sekolah. Sesi guru
  lain → 403. `staff_tu` tidak termasuk pembaca sesi (403 di endpoint sesi).
- `period_start`/`period_end` adalah nomor JP 1..20, `end >= start`; `topic`
  ≤200, `notes` ≤2000, `note` per record ≤500, maksimal 200 record per save.
- Kombinasi `(teaching_assignment, session_date, period_start)` unik → 409.
  Penugasan nonaktif tidak bisa dibuka sesi baru → 400.
- Simpan record hanya untuk siswa yang ada di roster sesi itu; siswa lain,
  status di luar enum, atau siswa duplikat → 400.
- Sesi yang sudah `closed` menolak simpan record **dan** penutupan ulang
  dengan 409; server juga menolak lewat trigger DB.
- Status record: `present`, `late`, `sick`, `excused`, `absent`.
- Audit log: `subject.create`, `subject.update`, `teaching_assignment.create`,
  `teaching_assignment.update`, `attendance_session.create`,
  `attendance_session.records`, `attendance_session.close`.

Objek `subject`:

```json
{
  "id": "uuid",
  "code": "MTK",
  "name": "Matematika",
  "description": "Wajib",
  "is_active": true,
  "created_at": "...",
  "updated_at": "..."
}
```

Objek `assignment`:

```json
{
  "id": "uuid",
  "teacher_id": "uuid",
  "teacher_name": "Budi Guru",
  "subject_id": "uuid",
  "subject_code": "MTK",
  "subject_name": "Matematika",
  "class_id": "uuid",
  "class_name": "12A",
  "is_active": true,
  "created_at": "...",
  "updated_at": "..."
}
```

Objek `session` (konteks penugasan sudah didenormalisasi):

```json
{
  "id": "uuid",
  "teaching_assignment_id": "uuid",
  "session_date": "2026-09-10",
  "period_start": 1,
  "period_end": 2,
  "topic": "Persamaan kuadrat",
  "notes": null,
  "status": "open",
  "teacher_id": "uuid",
  "teacher_name": "Budi Guru",
  "subject_id": "uuid",
  "subject_code": "MTK",
  "subject_name": "Matematika",
  "class_id": "uuid",
  "class_name": "12A",
  "closed_at": null,
  "created_at": "...",
  "updated_at": "..."
}
```

Roster row dan record:

```json
{
  "student_id": "uuid",
  "student_number": "SA-001",
  "full_name": "Ani Siswa",
  "enrollment_id": "uuid",
  "status": "present",
  "note": null
}
```

`status`/`note` bernilai `null` di roster selama sesi belum diisi. Record
menambahkan `id`, `session_id`, dan `updated_at`.

Rekap per penugasan (satu baris per penugasan yang punya sesi di rentang itu):

```json
{
  "teaching_assignment_id": "uuid",
  "class_id": "uuid",
  "class_name": "12A",
  "subject_id": "uuid",
  "subject_code": "MTK",
  "subject_name": "Matematika",
  "teacher_id": "uuid",
  "teacher_name": "Budi Guru",
  "sessions": 1,
  "records": 3,
  "present": 2,
  "late": 1,
  "sick": 0,
  "excused": 0,
  "absent": 0,
  "attendance_percentage": 100
}
```

`totals` memakai field yang sama tanpa identitas penugasan.
`attendance_percentage = (present + late) / records * 100` (2 desimal),
konsisten dengan laporan absensi harian.

### Tugas frontend

1. Halaman master data (admin + staff_tu): tab **Mata Pelajaran** (list,
   tambah/ubah, aktif/nonaktif) dan **Penugasan Mengajar** (guru × mapel ×
   kelas). Tampilkan pesan 409 spesifik untuk kode mapel atau triple penugasan
   duplikat.
2. Halaman **Presensi Mapel** untuk guru/duty/admin: filter tanggal + kelas +
   mapel + status, daftar sesi (`GET /attendance-sessions`), tombol buat sesi
   (pilih penugasan aktif, tanggal, JP awal–akhir, topik/catatan). Guru biasa
   hanya memilih penugasan miliknya; penugasan nonaktif tidak ditawarkan.
3. Detail sesi: roster dari `/roster`, pilih status per siswa (default
   `present` di UI boleh, tetapi server tetap menolak siswa di luar roster),
   simpan semua lewat `PUT .../records`, lalu tombol **Tutup Sesi** dengan
   konfirmasi. Setelah `status=closed`, nonaktifkan input dan tampilkan
   `closed_at`.
4. Tangani 409 dengan pesan berbeda: sesi duplikat ("Sesi untuk JP awal itu
   sudah ada") vs sesi tertutup ("Sesi sudah ditutup dan tidak bisa diubah").
5. Halaman **Rekap Mapel** memakai `/reports/subject-attendance`: tabel per
   kelas/mapel/guru + baris total, filter rentang tanggal/kelas/mapel/guru.
   Tampilkan `attendance_percentage` dengan dua desimal.
6. Semua label domain hard-coded bahasa Indonesia (tidak menambah key locale
   baru). Gating role di UI kosmetik; server tetap otoritatif.

### Kriteria terima

- Guru A tidak melihat penugasan/sesi guru B, dan membuka detail sesi guru B
  menampilkan state "tidak punya akses", bukan data.
- Input sesi tertutup nonaktif setelah reload; percobaan simpan tetap
  ditolak dengan pesan 409 yang spesifik.
- Sesi duplikat (penugasan + tanggal + JP awal sama) menampilkan pesan
  konflik spesifik.
- Rekap menampilkan total dan persentase yang sama dengan formula di atas.
- Tidak ada key locale baru.

---

## F11 — Ingest tap RFID

**Status backend:** diimplementasikan di wave ini (migration `000021_device_ingest`).
Melanjutkan registry F10: perangkat punya token, kartu siswa dipetakan ke UID,
dan reader mengirim batch tap.

### Kontrak backend

**Registry (admin)** — tambahan dari F10:

| Method | Path | Keterangan |
|--------|------|------------|
| POST | `/devices/:id/token` | rotasi token reader → 200 `{ "ingest_token": "dt_..." }` |

**Pemetaan kartu (admin, staff_tu):**

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/students/:id/card` | `{ "student_card": { "student_id": "uuid", "card_uid": "CARD-1001" \| null } }` |
| PUT | `/students/:id/card` | body `{ "card_uid": "CARD-1001" }` atau `{ "card_uid": null }` (hapus) → 200 `{ "student_card": {...} }` |

**Ingest (dipanggil reader, tanpa login user):**

| Method | Path | Keterangan |
|--------|------|------------|
| POST | `/device-taps` | header `X-Device-Token: dt_...`, body `{ "device_identifier": "RFID-GATE-01", "taps": [ { "card_uid": "CARD-1001", "occurred_at": "2026-09-12T07:01:00Z" } ] }` → 200 `{ "accepted": 2, "duplicates": 0, "unknown_cards": 1 }` |

**Log tap (admin, duty_teacher, principal):**

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/device-taps?device_id=&student_id=&card_uid=&from=YYYY-MM-DD&to=YYYY-MM-DD&limit=&offset=` | `{ "data": [tap], "total": n, "limit": 50, "offset": 0 }` |

Objek `tap`:

```json
{
  "id": "uuid",
  "device_id": "uuid",
  "device_name": "Reader Gerbang",
  "device_identifier": "RFID-GATE-01",
  "student_id": "uuid",
  "student_number": "S-1001",
  "student_name": "Budi Santoso",
  "card_uid": "CARD-1001",
  "occurred_at": "...",
  "created_at": "..."
}
```

`student_id`/`student_number`/`student_name` bernilai `null` bila kartu belum
dipetakan ke siswa.

Aturan ingest:

- Token dibuat saat `POST /devices` dan hanya ditampilkan sekali; server hanya
  menyimpan hash. Token hilang/dicuri → `POST /devices/:id/token`, token lama
  langsung tidak berlaku.
- Token tidak dikenal/`device_identifier` tidak cocok → 401
  `invalid_device_token`. Perangkat `inactive` → 403 `device_inactive`.
- `card_uid` dirapikan (trim + uppercase, maks 64 karakter, tanpa spasi);
  `occurred_at` RFC3339, boleh telat (reader offline), tetapi ditolak bila
  lebih dari 5 menit di masa depan.
- Batch 1–100 tap. Duplikat per `(device, card_uid, occurred_at)` diabaikan dan
  dihitung di `duplicates`, sehingga reader boleh mengulang batch.
- Ingest dibatasi rate limit per IP (default 600 request/menit, body maks
  128 KB). Melebihi batas → 429 `rate_limited` dengan header `Retry-After`;
  reader cukup mengirim ulang batch yang sama setelah menunggu (idempoten,
  baris lama dihitung sebagai `duplicates`). Beberapa reader di belakang satu
  NAT sekolah memakai kuota IP yang sama — naikkan
  `hardening.ingest_rate_limit` bila perlu.
- Tap dengan kartu tak dikenal tetap disimpan (`unknown_cards`), agar bisa
  ditelusuri lewat log. Pemetaan kartu berlaku untuk tap berikutnya; tap lama
  tidak di-backfill.
- Setiap batch sukses memperbarui `last_seen_at` perangkat.

### Tugas frontend

1. Setelah `POST /devices`, tampilkan `ingest_token` sekali di modal dengan
   tombol salin dan peringatan "token tidak dapat dilihat lagi".
2. Aksi "Rotasi token" di halaman perangkat (admin) dengan konfirmasi, lalu
   tampilkan token baru sekali.
3. Kolom `last_seen_at` pada tabel perangkat otomatis terisi setelah reader
   mengirim tap (sudah ada di F10).
4. Di detail siswa (admin/staff_tu), tambahkan field kartu: input UID, simpan,
   dan tombol hapus pemetaan. Tampilkan error 409 spesifik bila kartu sudah
   dipakai siswa lain.
5. Halaman/panel "Log tap" untuk admin, piket, dan principal: filter perangkat,
   rentang tanggal, dan UID kartu; baris tanpa siswa tampil sebagai
   "Kartu belum terdaftar" dengan UID-nya.
6. Live tap stream piket: polling `GET /device-taps?limit=20` (mis. tiap 5
   detik), dedupe berdasarkan `id`; tidak perlu websocket.

### Kriteria terima

- Token tidak pernah muncul di `GET /devices`; rotasi langsung mematikan token
  lama.
- Perangkat nonaktif berhenti menerima tap dengan pesan 403 yang jelas.
- Mengirim batch yang sama dua kali tidak menggandakan baris; UI menampilkan
  jumlah `accepted` dan `duplicates`.
- Tap kartu tak dikenal tetap terlihat di log dengan `student_id: null`.
- Tidak ada key locale baru.

---

## F12/F13 — Ketergantungan eksternal

- **F12 WhatsApp** membutuhkan pilihan provider (Meta Cloud API / Twilio) dan
  kredensial sebelum adapter dibuat.
- **F13 API publik + webhook** menunggu desain otorisasi API key.
