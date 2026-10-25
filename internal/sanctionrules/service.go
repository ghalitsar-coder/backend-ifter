package sanctionrules

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Service implements the sanction-rule use cases (handler -> usecase ->
// repository). Reads are open to every staff role that reaches
// groups.DisciplineRead (enforced at the router); every write is admin-only
// and additionally re-checked here (defense in depth, PRD §2.1). The
// no-overlapping-ranges invariant is enforced in this layer, not in the DB
// (package spec).
type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
	}
}

func requireAdmin(actor Actor) error {
	if !hasRole(actor.Roles, "admin") {
		return ErrForbidden
	}
	return nil
}

// List returns every rule (active and inactive) ordered by point ladder.
func (s *Service) List(ctx context.Context) (RuleList, error) {
	rows, err := s.queries.ListSanctionRules(ctx)
	if err != nil {
		return RuleList{}, fmt.Errorf("list sanction rules: %w", err)
	}
	items := make([]Rule, 0, len(rows))
	for _, row := range rows {
		items = append(items, ruleFromModel(row))
	}
	return RuleList{Data: items}, nil
}

// Get returns one rule for GET /sanction-rules/:id.
func (s *Service) Get(ctx context.Context, idValue string) (Rule, error) {
	id, err := parseUUID(idValue)
	if err != nil {
		return Rule{}, ErrInvalidInput
	}
	row, err := s.queries.GetSanctionRule(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, fmt.Errorf("get sanction rule: %w", err)
	}
	return ruleFromModel(row), nil
}
