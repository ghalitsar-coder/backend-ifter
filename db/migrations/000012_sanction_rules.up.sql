-- Sanction-rules package (docs/API_ADDENDUM.md §5/§6.4 + package spec, migration 000012).
-- Maps a student's accumulated discipline points to a sanction tier: each rule is
-- an inclusive point range [min_points, max_points] (max_points NULL = open-ended
-- top tier) carrying the action to take and the level label surfaced as
-- current_level on GET /discipline/students/:id.
--
-- The DB only enforces the well-formedness of a single range; overlapping ranges
-- across rules are rejected in the use case at the admin write surface (package
-- spec: "no overlapping ranges — enforce in use case, not DB").
CREATE TABLE sanction_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        varchar(100) NOT NULL,
  min_points  smallint NOT NULL CHECK (min_points >= 0),
  max_points  smallint,
  action      varchar(100) NOT NULL,
  level       varchar(30),
  description text,
  is_active   boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  -- Open-ended top tier: max_points NULL means "up to the ceiling". A stored
  -- range must span at least two point values (inclusive bounds), so min==max
  -- is rejected rather than degenerating into a single overlapping point.
  CONSTRAINT sanction_rules_range_check CHECK (min_points < COALESCE(max_points, 1000000000)),
  CONSTRAINT sanction_rules_name_not_blank CHECK (btrim(name) <> ''),
  CONSTRAINT sanction_rules_action_not_blank CHECK (btrim(action) <> ''),
  CONSTRAINT sanction_rules_level_max CHECK (level IS NULL OR btrim(level) <> ''),
  CONSTRAINT sanction_rules_description_max CHECK (description IS NULL OR char_length(description) <= 2000)
);

CREATE INDEX sanction_rules_min_points_idx ON sanction_rules (min_points, id);
CREATE INDEX sanction_rules_active_idx ON sanction_rules (is_active, min_points);
