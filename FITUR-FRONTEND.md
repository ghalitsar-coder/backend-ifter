# Fitur Frontend yang Perlu Menyusul Backend

Dokumen handoff untuk agen/AI yang mengerjakan repo frontend
(`frontend-ifter`). Window backend hanya membangun API; semua pekerjaan UI
dicatat di sini agar bisa dikerjakan paralel tanpa menebak kontrak.

Status terakhir: 2026-10-25.

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
| POST | `/devices` | body `{ name, identifier, device_type, location?, notes? }` → 201 `{ "device": {...} }` |
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
dihapus, cukup dinonaktifkan. `last_seen_at` belum diisi API ini — akan
di-update oleh ingest tap RFID (F11).

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

## F8 — Absensi per mata pelajaran (belum didesain backend)

**Status backend:** TBD. PRD hanya memberi petunjuk `attendance_sessions`.
Perlu tabel mata pelajaran, penugasan mengajar, sesi per JP, dan rekap.

Frontend nanti: layar input absensi per sesi, rekap per mapel, dan filter di
laporan. Kontrak final menyusul.

---

## F11/F12/F13 — Ketergantungan eksternal

- **F11 RFID tap** bergantung pada F10 (registry perangkat) dan spesifikasi
  payload reader. Endpoint ingest dan skema `last_seen_at` akan ditambahkan.
- **F12 WhatsApp** membutuhkan pilihan provider (Meta Cloud API / Twilio) dan
  kredensial sebelum adapter dibuat.
- **F13 API publik + webhook** menunggu desain otorisasi API key.

Frontend untuk F11 sudah sebagian ada (live tap stream piket); sinkronisasi
kontrak akan dicatat saat backend-nya dibangun.
