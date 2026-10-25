-- name: GetSchoolGeofence :one
SELECT id, latitude, longitude, radius_meters, is_enabled, updated_by, updated_at
FROM school_geofence
WHERE id = 1;

-- name: UpdateSchoolGeofence :one
UPDATE school_geofence
SET latitude = sqlc.arg(latitude),
    longitude = sqlc.arg(longitude),
    radius_meters = sqlc.arg(radius_meters),
    is_enabled = sqlc.arg(is_enabled),
    updated_by = sqlc.arg(updated_by),
    updated_at = now()
WHERE id = 1
RETURNING id, latitude, longitude, radius_meters, is_enabled, updated_by, updated_at;

-- name: GetTeacherPresenceByDate :one
SELECT id, teacher_id, presence_date, check_in_at, check_out_at,
       check_in_latitude, check_in_longitude, check_in_accuracy_meters,
       check_in_within_geofence, check_in_distance_meters,
       check_out_latitude, check_out_longitude, check_out_accuracy_meters,
       check_out_within_geofence, check_out_distance_meters,
       notes, created_at, updated_at
FROM teacher_presences
WHERE teacher_id = sqlc.arg(teacher_id) AND presence_date = sqlc.arg(presence_date);

-- name: CreateTeacherPresence :one
INSERT INTO teacher_presences (
  teacher_id, presence_date, check_in_at,
  check_in_latitude, check_in_longitude, check_in_accuracy_meters,
  check_in_within_geofence, check_in_distance_meters, notes
)
VALUES (
  sqlc.arg(teacher_id), sqlc.arg(presence_date), sqlc.arg(check_in_at),
  sqlc.arg(check_in_latitude), sqlc.arg(check_in_longitude), sqlc.arg(check_in_accuracy_meters),
  sqlc.arg(check_in_within_geofence), sqlc.arg(check_in_distance_meters),
  NULLIF(sqlc.arg(notes)::text, '')
)
RETURNING id, teacher_id, presence_date, check_in_at, check_out_at,
          check_in_latitude, check_in_longitude, check_in_accuracy_meters,
          check_in_within_geofence, check_in_distance_meters,
          check_out_latitude, check_out_longitude, check_out_accuracy_meters,
          check_out_within_geofence, check_out_distance_meters,
          notes, created_at, updated_at;

-- name: SetTeacherPresenceCheckOut :one
UPDATE teacher_presences
SET check_out_at = sqlc.arg(check_out_at),
    check_out_latitude = sqlc.arg(check_out_latitude),
    check_out_longitude = sqlc.arg(check_out_longitude),
    check_out_accuracy_meters = sqlc.arg(check_out_accuracy_meters),
    check_out_within_geofence = sqlc.arg(check_out_within_geofence),
    check_out_distance_meters = sqlc.arg(check_out_distance_meters),
    notes = COALESCE(NULLIF(sqlc.arg(notes)::text, ''), notes),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, teacher_id, presence_date, check_in_at, check_out_at,
          check_in_latitude, check_in_longitude, check_in_accuracy_meters,
          check_in_within_geofence, check_in_distance_meters,
          check_out_latitude, check_out_longitude, check_out_accuracy_meters,
          check_out_within_geofence, check_out_distance_meters,
          notes, created_at, updated_at;

-- name: ListTeacherPresences :many
SELECT id, teacher_id, presence_date, check_in_at, check_out_at,
       check_in_latitude, check_in_longitude, check_in_accuracy_meters,
       check_in_within_geofence, check_in_distance_meters,
       check_out_latitude, check_out_longitude, check_out_accuracy_meters,
       check_out_within_geofence, check_out_distance_meters,
       notes, created_at, updated_at
FROM teacher_presences
WHERE teacher_id = sqlc.arg(teacher_id)
  AND (sqlc.narg(from_date)::date IS NULL OR presence_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR presence_date <= sqlc.narg(to_date))
ORDER BY presence_date DESC, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountTeacherPresences :one
SELECT count(*)
FROM teacher_presences
WHERE teacher_id = sqlc.arg(teacher_id)
  AND (sqlc.narg(from_date)::date IS NULL OR presence_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR presence_date <= sqlc.narg(to_date));
