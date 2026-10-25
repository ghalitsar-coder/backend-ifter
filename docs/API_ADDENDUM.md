# API_ADDENDUM.md — Beyond-MVP Wave (RBAC + 7 Feature Packages)

> Contract file. **7 backend agents and 7 frontend agents code against this file
> verbatim.** Do not silently change paths, field names, role sets, or migration
> numbers — update this document in the same change and flag the impact.
>
> Scope: RBAC re-cut (6 roles) plus 7 new feature packages layered on the
> existing modular monolith (`handler -> usecase -> repository`, sqlc, gin).
> Base URL: `/api/v1`. Conventions follow `TECH_STACK.md` §9 and existing code.
>
> **Implementation status (wave 2, reconciled with code):** all 7 §6 packages
> are registered and live (`internal/transport/http/register_*.go`, wired from
> `New()` in `router.go`); migrations `000008`–`000015` exist in
> `db/migrations` (uncommitted on the wave-2 branch at the time of writing).
> Where the shipped surface deviates from the original contract (extra fields,
> extra endpoints, response keys), this document now describes **the code as it
> actually is** (re-checked task #29 against `internal/transport/http/` and the
> seven domain packages). Nullable fields serialize as JSON `null` except the
> student-account provisioning `user` object, where empty `email` /
> `last_login_at` are **omitted** entirely (`omitempty` tags).

---

## 0. Global conventions (apply to every endpoint below)

- **Auth**: `Authorization: Bearer <access_token>` (JWT). Refresh via HttpOnly
  cookie on `POST /api/v1/auth/refresh`. Same as today.
- **RBAC**: enforced at the router via `middleware.RequireRoles(...)` on the
  capability sub-group (see §2). Server-side data scoping (own-class vs
  school-wide) is enforced inside the use case — never trust a client id as proof
  of entitlement (`TECH_STACK.md` §7, PRD FR-A03).
- **JSON field naming**: `snake_case` everywhere, request and response.
- **List envelope**: `{ "data": [...], "total": <int>, "limit": <int>, "offset": <int> }`
  for paginated lists; `{ "data": [...] }` for unpaginated lists.
- **Single resource envelope**: the resource under its singular key, e.g.
  `{ "dispensation": {...} }` (matches existing `{"student": {...}}`, `{"class": {...}}`).
- **Error envelope** (unchanged, `internal/transport/http/response`):
  `{ "code": "...", "message": "...", "request_id": "..." }` with the status
  classes `400 invalid_request`, `401 unauthorized`, `403 forbidden`,
  `404 not_found`, `409 conflict`, `413 file_too_large`, `500 internal_error`.
- **Dates**: `YYYY-MM-DD` (string). **Timestamps**: RFC3339 with offset (string).
- **Status codes**: `200` read/update, `201` create, `204` delete/no-body.
- **Audit**: every sensitive write (verify/decision, sanction-rule change, point
  change, account provisioning, evidence add/delete, substitution) MUST write an
  `audit_logs` row (`action` = `<area>.<verb>`, before/after data, request_id).
  Never log tokens/passwords.
- **Body limits**: JSON writes reuse `middleware.BodyLimit(64<<10)`; the
  attendance bulk write uses `256<<10`. Uploads use `imports.max_file_size + 1MiB`
  (see §3, storage notes).

---

## 1. Role model (6 roles, `roles` table)

Existing codes: `admin`, `teacher`, `duty_teacher`, `principal`.
New codes seeded in **migration 000008**: `staff_tu`, `student`.

| Code | Name (id) | Capability summary |
| --- | --- | --- |
| `admin` | Administrator Sekolah | Everything today **plus** new write surfaces: sanction rules, geofence config, student accounts. |
| `staff_tu` | Staf Tata Usaha | Master data CRUD (students/teachers/classes/academic-years/terms/assignments), employment types, imports/exports/templates, reports read. **NOT** attendance/discipline recording, **NOT** user management, **NOT** settings. |
| `teacher` | Guru / Wali Kelas | Attendance ops (roster/bulk/close/corrections) for **own class**, discipline record/follow-up, dashboard (own class), GET + verify dispensations of own class, teacher presence self check-in/out. |
| `duty_teacher` | Guru Piket | Teacher capabilities but **school-wide for recording**, plus duty substitution management. |
| `principal` | Kepala Sekolah | **Read-only**: dashboard, attendance history/summary/reports/exports, discipline records/reports, dispensations list. No record/save/correction/verify writes. |
| `student` | Siswa | Only `/my/*` (own attendance, own discipline, own dispensations + create own request) and `auth/me`. **No dashboard/staff routes.** |

---

## 2. RBAC matrix (capability → role codes → `RouteGroups` field)

The router builds one authenticated parent and derives these guarded sub-groups.
Each is a field on the exported `RouteGroups` struct
(`internal/transport/http/router.go`); a feature register file attaches its
endpoints to the field(s) below and never touches auth middleware.

| Capability (`RouteGroups` field) | Role codes allowed | Purpose |
| --- | --- | --- |
| `Admin` | `admin` | User mgmt, attendance settings writes, holidays writes, duty-assignment master, discipline category mgmt + point overrides, audit logs, sanction-rule writes, geofence config write, student-account provisioning. |
| `MasterData` | `admin`, `staff_tu` | students/teachers/classes/academic-years/terms/assignments; employment types. |
| `AttendanceOps` | `admin`, `teacher`, `duty_teacher` | roster, bulk save, close, corrections, correction evidence, teacher presence check-in/out, duty substitutions, dispensation verify. Principal excluded. |
| `AttendanceRead` | `admin`, `teacher`, `duty_teacher`, `principal` | history, summary, record detail, attendance settings read, holidays read, geofence config read. |
| `DisciplineOps` | `admin`, `teacher`, `duty_teacher` | create record, add follow-up. |
| `DisciplineRead` | `admin`, `teacher`, `duty_teacher`, `principal` | categories, record list/detail, student summary, sanction-rule read. |
| `ReportsRead` | `admin`, `staff_tu`, `teacher`, `duty_teacher`, `principal` | dashboard, attendance/discipline reports + exports. |
| `Imports` | `admin`, `staff_tu` | Excel import jobs, templates, master exports. |
| `DispensationsRead` | `admin`, `staff_tu`, `teacher`, `duty_teacher`, `principal` | dispensation list/detail + private document download. |
| `My` | `student` | `/my/profile`, `/my/attendance*`, `/my/discipline*`, `/my/dispensations` (+ create). |
| `Authenticated` | any signed-in role | parent for the above. |
| `V1` | — | raw unauthenticated `/api/v1` (login/activate/refresh/logout; features that need a custom stack). |

`student` appears in **no** capability group except `My` (and `auth/me`, which is
`Authenticated`-only). `staff_tu` appears only in `MasterData`, `Imports`,
`ReportsRead`, `DispensationsRead`.

Beyond the `RouteGroups` fields, two role sets that no capability group carries
exactly are registered as dedicated `RequireRoles` sub-groups of `Authenticated`
(in `register_dispensations.go` and `register_duty_rotation.go`):

- `POST /dispensations` (staff create on behalf of a student): `admin`,
  `staff_tu`, `teacher`, `duty_teacher`.
- `POST|GET /uniform-reports`: `admin`, `staff_tu`, `duty_teacher`.

---

## 3. Existing endpoints re-cut (already implemented — reference only)

These are wired in `router.go` today; do **not** re-register them in a feature
file. Listed so agents know what already exists and under which group/roles.

**auth** (`V1`): `POST /auth/login`, `POST /auth/activate`, `POST /auth/refresh`,
`POST /auth/logout`, `GET /auth/me` (any authenticated role incl `student`).

**user mgmt / roles / audit** (`Admin`): `GET|POST /users`, `GET /users/:id`,
`PUT /users/:id/roles`, `PATCH /users/:id/status`,
`POST /users/:id/credential-reset`, `GET /roles`, `GET /audit-logs`.

**imports** (`Imports` = admin+staff_tu): `GET /imports`,
`POST /imports/students`, `POST /imports/teachers`, `GET /imports/:id`,
`POST /imports/:id/confirm`, `GET /import-templates/:entity`,
`GET /exports/students`, `GET /exports/teachers`.

**attendance ops** (`AttendanceOps`): `GET /attendance/roster`,
`POST /attendance/bulk`, `POST /attendance/close`,
`POST /attendance/records/:id/corrections`.

**attendance read** (`AttendanceRead`): `GET /attendance/history`,
`GET /attendance/summary`, `GET /attendance/records/:id`,
`GET /attendance/settings`, `GET /attendance/holidays`.

**attendance writes** (`Admin`): `PUT /attendance/settings`,
`POST /attendance/holidays`, `DELETE /attendance/holidays/:date`,
`GET|POST /duty-assignments`, `PUT /duty-assignments/:id/end`.

**discipline ops** (`DisciplineOps`): `POST /discipline/records`,
`POST /discipline/records/:id/followups`.

**discipline read** (`DisciplineRead`): `GET /discipline/categories`,
`GET /discipline/records`, `GET /discipline/records/:id`,
`GET /discipline/students/:id`.

**discipline admin writes** (`Admin`): `POST /discipline/categories`,
`PUT /discipline/categories/:id`, `PATCH /discipline/categories/:id/status`,
`PATCH /discipline/records/:id/points`.

**reports** (`ReportsRead`): `GET /dashboard`, `GET /exports/attendance`,
`GET /reports/attendance`, `GET /exports/attendance-report`,
`GET /exports/discipline`, `GET /reports/discipline`.

**master data** (`MasterData`): academic-years (+ nested terms), terms/:id,
classes (+ `/students`, `/homeroom`, `/status`), teachers (+ `/status`),
students (+ `/status`, `/class`).

**Storage/config (shared by imports + all uploads)**: private on-disk directory
`imports.dir` (`IFTER_IMPORTS_DIR`, default `./storage/imports`; mode `0700` dir /
`0600` file, **never** served as a static route). Max upload size
`imports.max_file_size` (`IFTER_IMPORTS_MAX_FILE_SIZE`, default 5 MiB). Document/
evidence uploads **reuse this same directory and size limit** — do not add a new
config key. Handler body limit for an upload route = `MaxFileSize() + 1<<20`.

---

## 4. Register-file contract (filled in — reference)

Each of the 7 features owns exactly one file under
`internal/transport/http/`. All seven are implemented and **called from
`New()`**; their domain services are built in `router.go` on the shared API
`pgxpool.Pool` when the readiness checker is that pool (tests with a fake
readiness leave `FeatureDeps` services nil, and a `Register*` whose service is
nil registers nothing). Implementation recap:

1. Create the domain package `internal/<area>/` following the existing
   handler→usecase→repository pattern (see `internal/attendance`, `internal/discipline`).
2. Add sqlc queries in `db/query/<area>.sql`, write the feature migration
   (number in §5), run `make sqlc` / sqlc generate.
3. In your `register_<feature>.go`, add a typed service field to `FeatureDeps`
   (router.go) **or** a dedicated parameter, build the handler struct
   (`new<Feature>Handler(service, log)` like `masterdata.go`), and register the
   routes onto the correct `RouteGroups` field(s). Keep write handlers using
   `middleware.CurrentUser` for the actor and pass `response.RequestID(c)` for audit.
4. Update `router_test.go` if you add routes asserted there.

**Exact scaffold signature (all 7 files, package `httptransport`):**

```go
func RegisterTeacherPresence(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterDispensations(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterDutyRotation(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterSanctionRules(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterEmploymentType(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterCorrectionEvidence(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
func RegisterStudentAccounts(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps)
```

`groups` fields you may attach to are named in each package below. All paths are
under `/api/v1`.

---

## 5. Migration numbering plan (assigned, do not reuse)

| Migration | Feature | New tables / key columns |
| --- | --- | --- |
| `000008` | **rbac_roles (DONE)** | seed `roles` rows `staff_tu`, `student`. No schema change. |
| `000009` | teacher-presence | `school_geofence` (single-row table, `id = 1` CHECK: latitude, longitude, radius_meters 1..20000, is_enabled bool default true, updated_by FK users, updated_at; seeded with placeholder coords). `teacher_presences` (id, teacher_id FK teachers, presence_date, check_in_at, check_out_at nullable, check_in_latitude/longitude/**check_in_accuracy_meters**, check_out_latitude/longitude/**check_out_accuracy_meters**, check_in_within_geofence bool, check_out_within_geofence bool nullable, check_in_distance_meters, check_out_distance_meters, notes nullable, created_at, updated_at); unique `(teacher_id, presence_date)`; check_out columns are all-or-nothing (`check_out_at` set together) and `check_out_at >= check_in_at`. |
| `000010` | dispensations | `dispensations` (id, student_id FK students, class_id FK classes nullable — snapshot of the student's enrollment taken at create time, type, reason, start_date, end_date nullable, status default `pending`, document_file_name nullable, **document_stored_key** nullable, submitted_by FK users, verified_by FK users nullable, verified_at nullable, note nullable, created_at, updated_at); unique `(student_id, type, start_date)` (duplicate request → `409`); file_name/stored_key pair together; verified_by/verified_at pair together. |
| `000011` | duty-rotation | `duty_rotations` (id, duty_assignment_id FK duty_assignments, substitute_teacher_id FK teachers, rotation_date, status `substituted|cancelled`, reason NOT NULL blank-checked ≤500 chars, created_by FK users, created_at, updated_at); unique `(duty_assignment_id, rotation_date, substitute_teacher_id)` (re-substitution of the same triple → `409`, kept even after cancellation). `uniform_reports` (id, for_date, teacher_id FK teachers, scope `student|teacher`, target_student_id FK students nullable — never set with scope `teacher`, note nullable ≤500 chars, created_at). |
| `000012` | sanction-rules | `sanction_rules` (id, name varchar(100), min_points smallint `>= 0`, max_points smallint nullable, action varchar(100), **level varchar(30) nullable** — tier label surfaced as `current_level`, description text nullable ≤2000 chars, is_active bool default true, created_at, updated_at); CHECK `min_points < COALESCE(max_points, ...)` (max_points NULL = open-ended top tier; min == max rejected). Overlapping ranges are rejected **in the use case** (`409`), not the DB. |
| `000013` | employment-type | `employment_types` (id, code UNIQUE varchar(30) format `^[a-z0-9_]+$`, name, description nullable, is_active bool, created_at, updated_at; seeded codes `permanent`, `honorary`, `staff_tu`); add `teachers.employment_type_id uuid NULL REFERENCES employment_types(id)`. |
| `000014` | correction-evidence | `attendance_correction_evidence` (id, attendance_record_id FK, correction_id FK attendance_corrections nullable, file_name, content_type, size_bytes bigint, stored_key, uploaded_by FK users, note nullable, created_at). Also adds `attendance_corrections.evidence_hash text` — the server-signed integrity hash produced by the correction flow. |
| `000015` | student-accounts | add `students.user_id uuid NULL UNIQUE REFERENCES users(id)` (login link used by `/my/*`; one login owns at most one student row). |

Seed data style mirrors `000002_identity` (`ON CONFLICT (code) DO NOTHING` for
roles). Feature migrations MUST be applied via `golang-migrate`; sqlc `schema:`
list in `sqlc.yaml` stays limited to table-creating migrations (000008 is
data-only, like 000007, and is intentionally omitted from that list).

---

## 6. Feature package contracts

### 6.1 Teacher presence — `register_teacher_presence.go` — migration `000009`

Domain package: `internal/teacherpresence`. Geofence decision is computed
**server-side** with Haversine against the school coords/radius stored in the
single-row `school_geofence` table (migration `000009`); the client never sends
a "valid" verdict. `latitude` and `longitude` are required in the body (an
explicit 0.0 is accepted — the handler uses pointer binding, so a missing
coordinate is `400 invalid_request`); `accuracy` and `notes` are optional.

Routes on `groups.AttendanceOps` (admin+teacher+duty_teacher) — a user only ever
records **their own** presence (use case resolves `teachers.user_id` = actor, 403
otherwise):

```
POST /api/v1/teacher-presence/check-in
  req  { "latitude": number, "longitude": number, "accuracy": number (optional),
         "check_time": "RFC3339 (optional; server-now if absent)",
         "notes": "..." (optional) }
  201  { "id": "...", "teacher_id": "...", "presence_date": "YYYY-MM-DD",
         "check_in_at": "RFC3339", "within_geofence": bool,
         "distance_meters": number, "message": "..." }
  (409 conflict when the teacher already has a presence for the day)

POST /api/v1/teacher-presence/check-out
  req  { "latitude": number, "longitude": number, "accuracy": number (optional),
         "check_time": "RFC3339 (optional)", "notes": "..." (optional) }
  201  { "id": "...", "teacher_id": "...", "presence_date": "YYYY-MM-DD",
         "check_in_at": "RFC3339", "check_out_at": "RFC3339",
         "duration_minutes": number, "check_out_within_geofence": bool,
         "check_out_distance_meters": number, "message": "..." }
```

Routes on `groups.AttendanceRead` (adds principal):

```
GET /api/v1/teacher-presence/today
  200  { "teacher_id": "...", "presence_date": "YYYY-MM-DD", "check_in_at": "RFC3339|null",
         "check_out_at": "RFC3339|null", "within_geofence": bool|null,
         "distance_meters": number|null }

GET /api/v1/teacher-presence/history?from=&to=&limit=&offset=
  200  { "data": [ { "id": "...", "teacher_id": "...", "presence_date": "YYYY-MM-DD",
                     "check_in_at": "RFC3339", "check_out_at": "RFC3339|null",
                     "duration_minutes": number, "within_geofence": bool,
                     "distance_meters": number } ],
         "total": int, "limit": int, "offset": int }
```

Geofence configuration (admin write surface) — this package owns the geofence
config referenced by the role model (stored in the single-row `school_geofence`
table):

```
GET /api/v1/geofence                                  (groups.AttendanceRead)
  200  { "latitude": number, "longitude": number, "radius_meters": number,
         "is_enabled": bool }

PUT /api/v1/geofence  { "latitude": number, "longitude": number,
                        "radius_meters": int, "is_enabled": bool }   (groups.Admin)
  200  same as GET ; audited (geofence.update)
  (all four fields are required — any missing field is 400 invalid_request;
   radius_meters must be 1..20000)
```

### 6.2 Dispensations — `register_dispensations.go` — migration `000010`

Domain package: `internal/dispensations`.
Enums — `type`: `sick|late|permit|other`; `status`: `pending|approved|rejected`.

Routes on `groups.DispensationsRead` (admin+staff_tu+teacher+duty_teacher+principal):

```
GET /api/v1/dispensations?status=&type=&student_id=&class_id=&from=&to=&limit=&offset=
  200  { "data": [ { "id": "...", "student_id": "...", "student_name": "...",
                     "class_id": "...", "class_name": "...", "type": "sick",
                     "reason": "...", "start_date": "YYYY-MM-DD",
                     "end_date": "YYYY-MM-DD|null", "status": "pending",
                     "document_available": bool, "submitted_by": "...",
                     "verified_by": "...|null", "verified_at": "RFC3339|null",
                     "note": "...|null", "created_at": "RFC3339" } ],
         "total": int, "limit": int, "offset": int }
  (limit default 50, cap 100; out-of-range limit/offset -> 400 invalid_request)

GET /api/v1/dispensations/:id
  200  { "dispensation": { ...same fields as an item above... } }

GET /api/v1/dispensations/:id/document
  200  binary; Content-Disposition attachment; X-Content-Type-Options: nosniff
  404  not_found when no document was attached (or the row does not exist /
       is not visible to the actor's data scope)
```

Staff create on behalf of a student — no capability group carries the exact
role set, so it runs on a dedicated `RequireRoles(admin, staff_tu, teacher,
duty_teacher)` sub-group of the authenticated parent:

```
POST /api/v1/dispensations
  req (multipart)  fields: student_id, type, reason, start_date, end_date
                   (optional); file part "document" (optional, image/pdf)
  req (json)       { "student_id": "...", "type": "sick", "reason": "...",
                     "start_date": "YYYY-MM-DD", "end_date": "YYYY-MM-DD" }
  201  { "dispensation": { ... } }
  (only the verify decision is audited (`dispensation.verify`); create writes
   no audit_logs row)
  409  conflict on a duplicate (student_id, type, start_date)
  413  file_too_large when the document exceeds the shared imports size limit
```

Routes on `groups.AttendanceOps` (admin+teacher+duty_teacher) — verify/decision,
teacher scoped to own class (active homeroom of the dispensation's class),
duty_teacher school-wide (existing service scoping):

```
POST /api/v1/dispensations/:id/verify  { "status": "approved|rejected", "note": "..." (optional) }
  200  { "dispensation": { ... } } ; audited (dispensation.verify)
  409  conflict when the dispensation is no longer `pending`
```

Routes on `groups.My` (student) — always resolved from the auth subject
(`students.user_id`), never from a client-supplied id:

```
GET  /api/v1/my/dispensations?status=&limit=&offset=
  200  { "data": [ ...item fields above... ], "total": int, "limit": int, "offset": int }

POST /api/v1/my/dispensations
  req (multipart)  fields: type, reason, start_date, end_date (optional); file part "document" (optional, image/pdf)
  req (json)       { "type": "sick", "reason": "...", "start_date": "YYYY-MM-DD", "end_date": "YYYY-MM-DD" }
  201  { "dispensation": { ... } }
```

Both `/my/dispensations` routes answer `403 forbidden` while no active student
row is linked to the signed-in login (`students.user_id`, migration `000015`);
the student is always resolved from the auth subject — a client-supplied
`student_id` on this route is ignored, never trusted. Duplicate
`(student, type, start_date)` is `409`.

Document upload for a dispensation reuses the imports storage dir + size limit
(§3).

### 6.3 Duty rotation — `register_duty_rotation.go` — migration `000011`

Domain package: `internal/dutyrotation`. Substitution surface plus the
uniform-note reports; the admin duty-assignment master CRUD already lives in
`router.go` on `Admin`.

Routes on `groups.AttendanceOps` (admin+teacher+duty_teacher):

```
GET /api/v1/duty-rotations?date=&weekday=
  200  { "data": [ { "id": "...", "duty_assignment_id": "...", "rotation_date": "YYYY-MM-DD",
                     "weekday": int, "original_teacher_id": "...", "original_teacher_name": "...",
                     "substitute_teacher_id": "...", "substitute_teacher_name": "...",
                     "status": "substituted|cancelled", "reason": "...", "created_at": "RFC3339" } ] }

POST /api/v1/duty-rotations
  req  { "duty_assignment_id": "...", "substitute_teacher_id": "...",
         "date": "YYYY-MM-DD", "reason": "..." }   (reason required, <= 500 chars)
  201  { "duty_rotation": { ...item fields... } } ; audited (duty.substitution)
  409  conflict on a duplicate (duty_assignment_id, date, substitute_teacher_id)

PATCH /api/v1/duty-rotations/:id/cancel
  req  { "reason": "..." (optional; blank keeps the stored reason) }
  200  { "duty_rotation": { ... } } ; audited (duty.substitution.cancel)
```

A `duty_teacher` may list/view substitutions relevant to them (own assignment or
own as substitute); admin sees all. `weekday` is 1..7 (Mon..Sun); an out-of-range
or non-numeric `weekday` is `400 invalid_request`.

Uniform reports (`uniform_reports`, same migration `000011`) run on a dedicated
`RequireRoles(admin, staff_tu, duty_teacher)` sub-group of the authenticated
parent (no capability group carries that exact set):

```
POST /api/v1/uniform-reports
  req  { "for_date": "YYYY-MM-DD", "teacher_id": "...", "scope": "student|teacher",
         "target_student_id": "..." (only with scope "student"),
         "note": "..." (optional, <= 500 chars) }
  201  { "uniform_report": { "id": "...", "for_date": "YYYY-MM-DD",
                             "teacher_id": "...", "scope": "student",
                             "target_student_id": "...|null",
                             "note": "...|null", "created_at": "RFC3339" } }
  (scope "teacher" never carries a target student — 400 if supplied)

GET /api/v1/uniform-reports?date=&teacher_id=
  200  { "data": [ ...uniform_report fields above... ] }
```

### 6.4 Sanction rules — `register_sanction_rules.go` — migration `000012`

Domain package: `internal/sanctionrules`. Maps accumulated discipline points to a
sanction/tier action. Writes admin-only; reads on `DisciplineRead`.

Routes on `groups.DisciplineRead` (admin+teacher+duty_teacher+principal):

```
GET /api/v1/sanction-rules
  200  { "data": [ { "id": "...", "name": "...", "min_points": int,
                     "max_points": int|null, "action": "...", "level": "...|null",
                     "description": "...|null", "is_active": bool,
                     "created_at": "RFC3339", "updated_at": "RFC3339" } ] }

GET /api/v1/sanction-rules/:id
  200  { "sanction_rule": { ...item fields... } }
```

Routes on `groups.Admin` (admin):

```
POST  /api/v1/sanction-rules   { "name","min_points","max_points"?,"action",
                                 "level"?,"description"?,"is_active"? }  -> 201 { "sanction_rule": {...} }
PUT   /api/v1/sanction-rules   { "rules": [ ...same body as POST... ] }  -> 200 { "data": [...items...] }
  (replace-all in one transaction, audited with before/after rule counts —
   package spec; the whole set is validated together, an overlap inside the
   payload is 409 and nothing is written)
PUT   /api/v1/sanction-rules/:id         { same as POST }                -> 200 { "sanction_rule": {...} }
PATCH /api/v1/sanction-rules/:id/status  { "is_active": bool } (required) -> 200 { "sanction_rule": {...} }
DELETE /api/v1/sanction-rules/:id                                         -> 204
```

`min_points` is required (an omitted or non-numeric value is `400`, even 0 must
be sent explicitly); `max_points` omitted/null = open-ended top tier. All writes audited (`sanction_rule.create|update|status|delete|replace`). Overlapping active point
ranges are rejected with `409 conflict` in the use case. The matching rule's
`level`/`action` is surfaced as `current_level` on
`GET /discipline/students/:id` (internal/discipline).

### 6.5 Employment type — `register_employment_type.go` — migration `000013`

Domain package: `internal/employmenttype`. Master data (admin+staff_tu).

Routes on `groups.MasterData` (admin+staff_tu):

```
GET   /api/v1/employment-types
  200  { "data": [ { "id": "...", "code": "...", "name": "...",
                     "description": "...|null", "is_active": bool,
                     "created_at": "RFC3339", "updated_at": "RFC3339" } ] }

GET   /api/v1/employment-types/:id        -> 200 { "employment_type": {...} }
POST  /api/v1/employment-types            { "code","name","description"?,"is_active"? } -> 201 { "employment_type": {...} }
PUT   /api/v1/employment-types/:id         { same as POST; omitted "is_active" keeps the row's current value }
                                          -> 200 { "employment_type": {...} }
PATCH /api/v1/employment-types/:id/status  { "is_active": bool } (required) -> 200 { "employment_type": {...} }
```

`code` must match `^[a-z0-9_]+$` (else `400 invalid_request`); a duplicate code
is `409 conflict` ("employment type code already exists"); `description` is
optional and capped at 2000 chars.

Migration also adds `teachers.employment_type_id`. Once present, the existing
`MasterData` teacher responses (`GET /teachers`, `GET /teachers/:id`, POST/PUT)
expose `employment_type_id` (nullable) and `employment_type_name` (nullable) in
snake_case; the teacher create/update request accepts optional `employment_type_id`.
Writes audited (`employment_type.create|update|status`).

### 6.6 Correction evidence — `register_correction_evidence.go` — migration `000014`

Domain package: `internal/correctionevidence` (or extend `internal/attendance`).
Attaches supporting documents to an attendance correction. Follows the same role
set as `POST /attendance/records/:id/corrections` (AttendanceOps; principal excluded).

Routes on `groups.AttendanceOps` (admin+teacher+duty_teacher):

```
POST /api/v1/attendance/records/:id/corrections/evidence   (multipart; body limit = MaxFileSize()+1MiB)
  form  file (image/pdf; file part name "file", required),
        note (optional), correction_id (optional — attach to a specific correction row)
  201  { "evidence": { "id": "...", "attendance_record_id": "...", "correction_id": "...|null",
                       "file_name": "...", "content_type": "...", "size_bytes": int,
                       "note": "...|null", "uploaded_by": "...", "created_at": "RFC3339" } } ; audited
        (stored_key is DB-internal and never exposed in the response)

GET /api/v1/attendance/records/:id/corrections/evidence
  200  { "data": [ ...evidence fields above (no binary)... ] }

GET /api/v1/attendance/corrections/evidence/:evidenceId/download
  200  binary; Content-Disposition attachment; X-Content-Type-Options: nosniff;
       Cache-Control: no-store

DELETE /api/v1/attendance/corrections/evidence/:evidenceId
  204 ; audited
```

Storage: reuse `imports.dir` + `imports.max_file_size` (§3). Validate extension +
content-type (image/pdf); reject oversize with `413 file_too_large`. Files stored
with an opaque key under `stored_key`, never enumerable via a static route.

### 6.7 Student accounts — `register_student_accounts.go` — migration `000015`

Domain package: `internal/studentaccounts`. Two surfaces: admin provisioning of a
student's login account, and the student's own `/my/*` portal (own profile, own
attendance + summary, own discipline + summary — list routes plus the
`/my/attendance-history` alias). The student's own **dispensations** are in
§6.2, not here.

Routes on `groups.Admin` (admin only — `staff_tu` is deliberately excluded):

```
POST /api/v1/students/:id/account
  req  { "username": "..." (optional; defaults to the student's student_number),
         "email": "..." (optional) }   (an absent/empty body is valid)
  201  { "user": { "id": "...", "username": "...", "full_name": "...",
                   "status": "pending", "roles": ["student"], "created_at": "RFC3339",
                   "email": "..." (omitted when not set),
                   "last_login_at": "RFC3339" (omitted until first login) },
         "credential": { "token": "...", "purpose": "invite", "expires_at": "RFC3339" } } ; audited
  (reuses the auth invite/credential flow — POST /users machinery; no password
   is ever issued here, `full_name` always comes from the student row. Links
   the created user via students.user_id; the student activates via
   POST /auth/activate.)
  409  conflict — student already has a login account / student is not active /
       username or email already taken

GET /api/v1/students/:id/account
  200  { "account": { "user_id": "...|null", "username": "...|null",
                      "status": "...|null", "last_login_at": "RFC3339|null" } }
  (`status` is the linked login user's status — pending/active/inherited;
   all fields are null when no account is linked; 404 when the student row
   does not exist)

DELETE /api/v1/students/:id/account
  204 ; deactivates the login user (status inactive + revoke sessions and unused
       invites + clear the students.user_id link); student record and history
       are preserved. ; audited (student_account.delete)
```

Routes on `groups.My` (student) — resolved from `students.user_id` = actor;
`403 forbidden` ("no active student record is linked to this login account")
while the login has no active linked student:

```
GET /api/v1/my/profile
  200  { "profile": { "student_id": "...", "student_number": "...",
                      "full_name": "...", "class_id": "...|null",
                      "class_name": "...|null" } }
  (the student id a portal needs — GET /auth/me does not carry it)

GET /api/v1/my/attendance?from=&to=&limit=&offset=
GET /api/v1/my/attendance-history?from=&to=&limit=&offset=   (alias, same handler)
  200  { "data": [ { "date": "YYYY-MM-DD", "status": "present|late|sick|excused|absent",
                     "check_in_time": "RFC3339|null", "class_id": "...",
                     "class_name": "..." } ],
         "total": int, "limit": int, "offset": int }

GET /api/v1/my/attendance-summary?from=&to=
  200  { "from": "YYYY-MM-DD", "to": "YYYY-MM-DD",
         "data": [ { "student_id": "...", "student_number": "...", "student_name": "...",
                     "class_id": "...", "class_name": "...", "total_days": int,
                     "present": int, "late": int, "sick": int, "excused": int,
                     "absent": int, "attendance_percentage": number } ],
         "totals": { "students": int, "total_days": int, "present": int, "late": int,
                     "sick": int, "excused": int, "absent": int,
                     "attendance_percentage": number } }

GET /api/v1/my/discipline?from=&to=&limit=&offset=
  200  { "data": [ { "id": "...", "occurred_at": "RFC3339", "category_name": "...",
                     "description": "...", "points": int,
                     "status": "new|under_review|in_followup|completed|cancelled" } ],
         "total": int, "limit": int, "offset": int }

GET /api/v1/my/discipline-summary?from=&to=
  200  { "student_id": "...", "student_number": "...", "student_name": "...",
         "total_points": int, "total_records": int,
         "categories": [ { "category_id": "...", "category_name": "...",
                           "count": int, "points": int } ],
         "current_level": { "level": "...|null", "action": "..." } | null }
```

All `/my/*` list endpoints: `limit` default 50, cap 100; out-of-range values are
`400 invalid_request`. `GET /students/:id` carries no `has_user_account` flag —
use `GET /students/:id/account` instead.

---

## 7. Deviations / notes for implementers

- Principal is intentionally tightened from the old catch-all `staff` group:
  principal now reaches read groups only (AttendanceRead, DisciplineRead,
  ReportsRead, DispensationsRead), never roster/bulk/close/corrections/verify/record/followup.
- `GET /attendance/roster` was moved to `AttendanceOps` (it is the recording
  screen), so principal does not get it — matches "read-only history/summary/
  reports/exports" for principal.
- `staff_tu` reaches `ReportsRead` (incl `GET /dashboard`) as "reports read";
  it does **not** reach per-record attendance/discipline detail or any ops group.
- The `GET /duty-assignments` master schedule remains `Admin`; `duty_teacher`
  substitution is the separate duty-rotation package (§6.3).
- `/my/*` and the student `/my/dispensations` create endpoint are the only
  `student` routes besides `auth/me`.
