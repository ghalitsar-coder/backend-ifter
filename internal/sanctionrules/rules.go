package sanctionrules

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// ruleRange is a validated inclusive point interval [min, max] used for the
// overlap check; a NULL max_points is stored as openEndedMax so the top tier
// collides with anything above its floor.
type ruleRange struct {
	min int
	max int
}

func (r ruleRange) overlaps(other ruleRange) bool {
	return r.min <= other.max && other.min <= r.max
}

// normalized is a validated RuleInput, ready for the sqlc params plus the
// range used for overlap detection and the effective is_active flag.
type normalized struct {
	name        string
	minPoints   int16
	maxPoints   pgtype.Int2
	action      string
	level       pgtype.Text
	description pgtype.Text
	active      bool
	rng         ruleRange
}

func normalizeRuleInput(in RuleInput) (normalized, error) {
	action, err := cleanText(in.Action, maxActionRunes)
	if err != nil || action == "" {
		return normalized{}, ErrInvalidInput
	}
	name, err := cleanText(in.Name, maxNameRunes)
	if err != nil {
		return normalized{}, ErrInvalidInput
	}
	if name == "" {
		// The replace-all payload of the package spec carries no name; fall
		// back to the action text so the NOT NULL column stays meaningful.
		name = action
	}
	level, err := cleanText(in.Level, maxLevelRunes)
	if err != nil {
		return normalized{}, ErrInvalidInput
	}
	description, err := cleanText(in.Description, maxDescriptionRunes)
	if err != nil {
		return normalized{}, ErrInvalidInput
	}
	if in.MinPoints == nil || *in.MinPoints < 0 || *in.MinPoints > maxPoints {
		return normalized{}, ErrInvalidInput
	}
	min := *in.MinPoints
	result := normalized{
		name:        name,
		minPoints:   int16(min),
		action:      action,
		level:       optionalText(level),
		description: optionalText(description),
		active:      true,
		rng:         ruleRange{min: min, max: openEndedMax},
	}
	if in.MaxPoints != nil {
		// The migration CHECK requires min < max (inclusive bounds must span
		// at least two point values); validate the same rule here.
		if *in.MaxPoints < 0 || *in.MaxPoints > maxPoints || *in.MaxPoints <= min {
			return normalized{}, ErrInvalidInput
		}
		result.maxPoints = pgtype.Int2{Int16: int16(*in.MaxPoints), Valid: true}
		result.rng.max = *in.MaxPoints
	}
	if in.IsActive != nil {
		result.active = *in.IsActive
	}
	return result, nil
}

// rangeOverlapsActive reports whether candidate collides with any of the
// already-active ranges, excluding the rule id being edited.
func rangeOverlapsActive(rows []db.ListActiveSanctionRuleRangesForUpdateRow, candidate ruleRange, exclude pgtype.UUID) bool {
	for _, row := range rows {
		if exclude.Valid && row.ID.Valid && row.ID.Bytes == exclude.Bytes {
			continue
		}
		max := openEndedMax
		if row.MaxPoints.Valid {
			max = int(row.MaxPoints.Int16)
		}
		if candidate.overlaps(ruleRange{min: int(row.MinPoints), max: max}) {
			return true
		}
	}
	return false
}

func (n normalized) createParams() db.CreateSanctionRuleParams {
	return db.CreateSanctionRuleParams{
		Name:        n.name,
		MinPoints:   n.minPoints,
		MaxPoints:   n.maxPoints,
		Action:      n.action,
		Level:       n.level,
		Description: n.description,
	}
}

func (n normalized) updateParams(id pgtype.UUID) db.UpdateSanctionRuleParams {
	return db.UpdateSanctionRuleParams{
		ID:          id,
		Name:        n.name,
		MinPoints:   n.minPoints,
		MaxPoints:   n.maxPoints,
		Action:      n.action,
		Level:       n.level,
		Description: n.description,
	}
}

// Create stores a new rule (POST /sanction-rules). New rules are always
// active; deactivation goes through PATCH /sanction-rules/:id/status. The
// point range must not overlap any other active rule (409). Audited as
// sanction_rule.create.
func (s *Service) Create(ctx context.Context, actor Actor, in RuleInput, requestID string) (Rule, error) {
	if err := requireAdmin(actor); err != nil {
		return Rule{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Rule{}, ErrUnauthorized
	}
	n, err := normalizeRuleInput(in)
	if err != nil {
		return Rule{}, err
	}
	var created Rule
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		rows, err := q.ListActiveSanctionRuleRangesForUpdate(ctx)
		if err != nil {
			return fmt.Errorf("lock active sanction rule ranges: %w", err)
		}
		if rangeOverlapsActive(rows, n.rng, pgtype.UUID{}) {
			return ErrConflict
		}
		params := n.createParams()
		params.IsActive = pgtype.Bool{Bool: true, Valid: true}
		row, err := q.CreateSanctionRule(ctx, params)
		if err != nil {
			return mapWriteError(fmt.Errorf("create sanction rule: %w", err))
		}
		created = ruleFromModel(row)
		return s.auditData(ctx, q, actorID, "sanction_rule.create", row.ID, nil, newRuleView(created), requestID)
	})
	if err != nil {
		return Rule{}, err
	}
	return created, nil
}

// Update replaces a rule's payload (PUT /sanction-rules/:id); is_active is
// not part of this surface (PATCH :id/status owns it). The new range must not
// overlap another active rule (409). Audited as sanction_rule.update with
// before/after snapshots.
func (s *Service) Update(ctx context.Context, actor Actor, idValue string, in RuleInput, requestID string) (Rule, error) {
	if err := requireAdmin(actor); err != nil {
		return Rule{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Rule{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Rule{}, ErrInvalidInput
	}
	n, err := normalizeRuleInput(in)
	if err != nil {
		return Rule{}, err
	}
	var updated Rule
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetSanctionRuleForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock sanction rule: %w", err)
		}
		if before.IsActive {
			rows, err := q.ListActiveSanctionRuleRangesForUpdate(ctx)
			if err != nil {
				return fmt.Errorf("lock active sanction rule ranges: %w", err)
			}
			if rangeOverlapsActive(rows, n.rng, id) {
				return ErrConflict
			}
		}
		row, err := q.UpdateSanctionRule(ctx, n.updateParams(id))
		if err != nil {
			return mapWriteError(fmt.Errorf("update sanction rule: %w", err))
		}
		updated = ruleFromModel(row)
		return s.auditData(ctx, q, actorID, "sanction_rule.update", id,
			newRuleView(ruleFromModel(before)), newRuleView(updated), requestID)
	})
	if err != nil {
		return Rule{}, err
	}
	return updated, nil
}

// SetStatus toggles is_active (PATCH /sanction-rules/:id/status). Activating
// a rule re-runs the overlap check against the other active rules (409).
// Audited as sanction_rule.status.
func (s *Service) SetStatus(ctx context.Context, actor Actor, idValue string, active bool, requestID string) (Rule, error) {
	if err := requireAdmin(actor); err != nil {
		return Rule{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Rule{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Rule{}, ErrInvalidInput
	}
	var updated Rule
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetSanctionRuleForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock sanction rule: %w", err)
		}
		if active && !before.IsActive {
			rows, err := q.ListActiveSanctionRuleRangesForUpdate(ctx)
			if err != nil {
				return fmt.Errorf("lock active sanction rule ranges: %w", err)
			}
			max := openEndedMax
			if before.MaxPoints.Valid {
				max = int(before.MaxPoints.Int16)
			}
			if rangeOverlapsActive(rows, ruleRange{min: int(before.MinPoints), max: max}, id) {
				return ErrConflict
			}
		}
		row, err := q.SetSanctionRuleStatus(ctx, db.SetSanctionRuleStatusParams{
			IsActive: active,
			ID:       id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("set sanction rule status: %w", err))
		}
		updated = ruleFromModel(row)
		return s.auditData(ctx, q, actorID, "sanction_rule.status", id,
			newRuleView(ruleFromModel(before)), newRuleView(updated), requestID)
	})
	if err != nil {
		return Rule{}, err
	}
	return updated, nil
}

// Delete removes one rule (DELETE /sanction-rules/:id -> 204). Audited as
// sanction_rule.delete with the removed snapshot as before data.
func (s *Service) Delete(ctx context.Context, actor Actor, idValue string, requestID string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return ErrInvalidInput
	}
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetSanctionRuleForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock sanction rule: %w", err)
		}
		if _, err := q.DeleteSanctionRule(ctx, id); err != nil {
			return mapWriteError(fmt.Errorf("delete sanction rule: %w", err))
		}
		return s.auditData(ctx, q, actorID, "sanction_rule.delete", id,
			newRuleView(ruleFromModel(before)), nil, requestID)
	})
}

// ReplaceAll swaps the entire rule set in one transaction
// (PUT /sanction-rules {rules:[...]}): delete-all + insert-all, with the
// submitted active ranges validated for overlap first (409). Audited as
// sanction_rule.replace with before/after rule counts (package spec). An
// empty rules list clears every rule.
func (s *Service) ReplaceAll(ctx context.Context, actor Actor, inputs []RuleInput, requestID string) (RuleList, error) {
	if err := requireAdmin(actor); err != nil {
		return RuleList{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RuleList{}, ErrUnauthorized
	}
	rules := make([]normalized, 0, len(inputs))
	var activeRanges []ruleRange
	for _, in := range inputs {
		n, err := normalizeRuleInput(in)
		if err != nil {
			return RuleList{}, err
		}
		if n.active {
			for _, prev := range activeRanges {
				if prev.overlaps(n.rng) {
					return RuleList{}, ErrConflict
				}
			}
			activeRanges = append(activeRanges, n.rng)
		}
		rules = append(rules, n)
	}
	var result RuleList
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.CountSanctionRules(ctx)
		if err != nil {
			return fmt.Errorf("count sanction rules before replace: %w", err)
		}
		if err := q.DeleteAllSanctionRules(ctx); err != nil {
			return fmt.Errorf("delete sanction rules: %w", err)
		}
		for _, n := range rules {
			params := n.createParams()
			params.IsActive = pgtype.Bool{Bool: n.active, Valid: true}
			if _, err := q.CreateSanctionRule(ctx, params); err != nil {
				return mapWriteError(fmt.Errorf("insert sanction rule: %w", err))
			}
		}
		after, err := q.CountSanctionRules(ctx)
		if err != nil {
			return fmt.Errorf("count sanction rules after replace: %w", err)
		}
		if err := s.auditData(ctx, q, actorID, "sanction_rule.replace", pgtype.UUID{},
			map[string]any{"rule_count": before}, map[string]any{"rule_count": after}, requestID); err != nil {
			return err
		}
		rows, err := q.ListSanctionRules(ctx)
		if err != nil {
			return fmt.Errorf("list sanction rules after replace: %w", err)
		}
		items := make([]Rule, 0, len(rows))
		for _, row := range rows {
			items = append(items, ruleFromModel(row))
		}
		result = RuleList{Data: items}
		return nil
	})
	if err != nil {
		return RuleList{}, err
	}
	return result, nil
}
