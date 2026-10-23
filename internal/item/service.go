package item

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Item, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return Item{}, ErrInvalidName
	}

	created, err := s.repo.Create(ctx, name)
	if err != nil {
		return Item{}, fmt.Errorf("create item: %w", err)
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Item, error) {
	found, err := s.repo.Get(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("get item: %w", err)
	}
	return found, nil
}

func (s *Service) List(ctx context.Context, in ListInput) (ListResult, error) {
	page := in.Page
	if page < 1 {
		page = 1
	}
	if page > MaxPage {
		page = MaxPage
	}

	perPage := in.PerPage
	if perPage < 1 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}

	items, err := s.repo.List(ctx, int32(perPage), int32((page-1)*perPage))
	if err != nil {
		return ListResult{}, fmt.Errorf("list items: %w", err)
	}
	total, err := s.repo.Count(ctx)
	if err != nil {
		return ListResult{}, fmt.Errorf("count items: %w", err)
	}

	return ListResult{Items: items, Page: page, PerPage: perPage, Total: total}, nil
}
