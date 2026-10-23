package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ghalitsar-coder/backend-ifter/internal/item"
)

type fakeItemRepository struct {
	createFn func(ctx context.Context, name string) (item.Item, error)
	getFn    func(ctx context.Context, id uuid.UUID) (item.Item, error)
	listFn   func(ctx context.Context, limit, offset int32) ([]item.Item, error)
	countFn  func(ctx context.Context) (int64, error)
}

func (f *fakeItemRepository) Create(ctx context.Context, name string) (item.Item, error) {
	return f.createFn(ctx, name)
}

func (f *fakeItemRepository) Get(ctx context.Context, id uuid.UUID) (item.Item, error) {
	return f.getFn(ctx, id)
}

func (f *fakeItemRepository) List(ctx context.Context, limit, offset int32) ([]item.Item, error) {
	return f.listFn(ctx, limit, offset)
}

func (f *fakeItemRepository) Count(ctx context.Context) (int64, error) {
	return f.countFn(ctx)
}

func itemRouter(repo item.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := newItemHandler(item.NewService(repo), testLogger())

	router := gin.New()
	router.POST("/items", handler.Create)
	router.GET("/items", handler.List)
	router.GET("/items/:id", handler.Get)
	return router
}

func TestItemHandlerCreate(t *testing.T) {
	repo := &fakeItemRepository{
		createFn: func(_ context.Context, name string) (item.Item, error) {
			return item.Item{ID: uuid.New(), Name: name}, nil
		},
	}
	router := itemRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body struct {
		Data item.Item `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Name != "hello" {
		t.Errorf("data.name = %q, want hello", body.Data.Name)
	}
}

func TestItemHandlerCreateRejectsInvalidBody(t *testing.T) {
	// createFn is nil: invalid input must be rejected before persistence.
	router := itemRouter(&fakeItemRepository{})

	for _, payload := range []string{`{"name":""}`, `{"name":"   "}`, `not-json`} {
		req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("payload %q status = %d, want %d", payload, rec.Code, http.StatusUnprocessableEntity)
		}
	}
}

func TestItemHandlerGet(t *testing.T) {
	repo := &fakeItemRepository{
		getFn: func(_ context.Context, id uuid.UUID) (item.Item, error) {
			return item.Item{}, pgx.ErrNoRows
		},
	}
	router := itemRouter(repo)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items/"+uuid.NewString(), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items/not-a-uuid", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed id status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestItemHandlerListMeta(t *testing.T) {
	repo := &fakeItemRepository{
		listFn: func(context.Context, int32, int32) ([]item.Item, error) {
			return []item.Item{{ID: uuid.New(), Name: "one"}}, nil
		},
		countFn: func(context.Context) (int64, error) { return 7, nil },
	}
	router := itemRouter(repo)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/items?page=2&per_page=2", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Data []item.Item `json:"data"`
		Meta struct {
			Page    int   `json:"page"`
			PerPage int   `json:"per_page"`
			Total   int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Meta.Page != 2 || body.Meta.PerPage != 2 || body.Meta.Total != 7 {
		t.Errorf("meta = %+v, want page=2 per_page=2 total=7", body.Meta)
	}
}
