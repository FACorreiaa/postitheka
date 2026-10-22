package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/media"
)

// uploadTTL is how long a presigned upload URL stays valid. Long enough for a
// large video on a phone connection, short enough that a leaked URL is not a
// standing write grant into the bucket.
const uploadTTL = 30 * time.Minute

type Upload struct {
	MediaID     uuid.UUID
	URL         string
	ContentType string
	ExpiresAt   time.Time
}

// ReserveUpload records a pending upload and returns where to PUT it.
func (s *Service) ReserveUpload(ctx context.Context, userID uuid.UUID, contentType string, size int64) (Upload, error) {
	if err := s.registry.AcceptsUpload(contentType, size); err != nil {
		return Upload{}, &BadRequest{Message: err.Error()}
	}

	id := uuid.New()
	// Keys are namespaced by user, so a bucket listing or a lifecycle rule
	// can be scoped to one person's files.
	key := fmt.Sprintf("uploads/%s/%s", userID, media.Filename(id, contentType))

	if err := s.q.CreateMedia(ctx, db.CreateMediaParams{
		ID:          id,
		UserID:      userID,
		ObjectKey:   key,
		ContentType: contentType,
		SizeBytes:   size,
	}); err != nil {
		return Upload{}, fmt.Errorf("reserve upload: %w", err)
	}

	url, err := s.storage.PresignUpload(ctx, key, uploadTTL)
	if err != nil {
		return Upload{}, err
	}
	return Upload{MediaID: id, URL: url, ContentType: contentType, ExpiresAt: s.now().Add(uploadTTL)}, nil
}
