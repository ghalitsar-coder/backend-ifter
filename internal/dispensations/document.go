package dispensations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// DocumentDownload serves the stored document (GET /dispensations/:id/document).
// The file lives in the private imports storage dir and is only reachable
// through this scoped endpoint — never via a static route.
func (s *Service) Document(ctx context.Context, actor Actor, id string) (DocumentDownload, error) {
	scope, err := s.resolveReadScope(ctx, actor)
	if err != nil {
		return DocumentDownload{}, err
	}
	dispensationID, err := parseUUID(id)
	if err != nil {
		return DocumentDownload{}, ErrInvalidInput
	}
	row, err := s.queries.GetDispensation(ctx, dispensationID)
	if err != nil {
		if database.IsNotFound(err) {
			return DocumentDownload{}, ErrNotFound
		}
		return DocumentDownload{}, fmt.Errorf("get dispensation: %w", err)
	}
	if err := checkClassAccess(scope, row.ClassID); err != nil {
		return DocumentDownload{}, err
	}
	if !row.DocumentStoredKey.Valid {
		return DocumentDownload{}, ErrNotFound
	}
	data, err := s.readFile(row.DocumentStoredKey.String)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DocumentDownload{}, ErrNotFound
		}
		return DocumentDownload{}, fmt.Errorf("read dispensation document: %w", err)
	}
	filename := row.DocumentFileName.String
	if filename == "" {
		filename = "document" + extensionOf(row.DocumentStoredKey.String)
	}
	return DocumentDownload{
		Filename:    filename,
		ContentType: contentTypeForExtension(extensionOf(row.DocumentStoredKey.String)),
		Data:        data,
	}, nil
}
