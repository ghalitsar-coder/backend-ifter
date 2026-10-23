package item

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeRepository struct {
	createFn func(ctx context.Context, name string) (Item, error)
	getFn    func(ctx context.Context, id uuid.UUID) (Item, error)
	listFn   func(ctx context.Context, limit, offset int32) ([]Item, error)
	countFn  func(ctx context.Context) (int64, error)
}

func (f *fakeRepository) Create(ctx context.Context, name string) (Item, error) {
	return f.createFn(ctx, name)
}

func (f *fakeRepository) Get(ctx context.Context, id uuid.UUID) (Item, error) {
	return f.getFn(ctx, id)
}

func (f *fakeRepository) List(ctx context.Context, limit, offset int32) ([]Item, error) {
	return f.listFn(ctx, limit, offset)
}

func (f *fakeRepository) Count(ctx context.Context) (int64, error) {
	return f.countFn(ctx)
}

func TestCreateValidatesName(t *testing.T) {
	// createFn is nil on purpose: validation must fail before the repository is
	// touched.
	svc := NewService(&fakeRepository{})

	for _, name := range []string{"", "   ", strings.Repeat("a", MaxNameLength+1)} {
		if _, err := svc.Create(context.Background(), CreateInput{Name: name}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestCreateTrimsName(t *testing.T) {
	var received string
	svc := NewService(&fakeRepository{
		createFn: func(_ context.Context, name string) (Item, error) {
			received = name
			return Item{ID: uuid.New(), Name: name}, nil
		},
	})

	if _, err := svc.Create(context.Background(), CreateInput{Name: "  hello  "}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if received != "hello" {
		t.Errorf("repository received %q, want hello", received)
	}
}

func TestGetMapsNotFound(t *testing.T) {
	svc := NewService(&fakeRepository{
		getFn: func(context.Context, uuid.UUID) (Item, error) {
			return Item{}, pgx.ErrNoRows
		},
	})

	if _, err := svc.Get(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestListClampsPagination(t *testing.T) {
	tests := []struct {
		name                  string
		page, perPage         int
		wantPage, wantPerPage int
		wantLimit, wantOffset int32
	}{
		{"defaults", 0, 0, 1, DefaultPerPage, DefaultPerPage, 0},
		{"caps per page", 2, 500, 2, MaxPerPage, MaxPerPage, MaxPerPage},
		{"caps page", MaxPage + 10, 20, MaxPage, 20, 20, int32((MaxPage - 1) * 20)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit, gotOffset int32
			svc := NewService(&fakeRepository{
				listFn: func(_ context.Context, limit, offset int32) ([]Item, error) {
					gotLimit, gotOffset = limit, offset
					return []Item{}, nil
				},
				countFn: func(context.Context) (int64, error) { return 0, nil },
			})

			result, err := svc.List(context.Background(), ListInput{Page: tt.page, PerPage: tt.perPage})
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if result.Page != tt.wantPage || result.PerPage != tt.wantPerPage {
				t.Errorf("page/perPage = %d/%d, want %d/%d", result.Page, result.PerPage, tt.wantPage, tt.wantPerPage)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}
