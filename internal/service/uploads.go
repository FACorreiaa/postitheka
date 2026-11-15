package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
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

// VideoFacts is what a client read from a video before uploading it: the
// server has no way to measure a file (VID-001). Zero means not known.
type VideoFacts struct {
	DurationMS    int
	Width, Height int
}

// Bounds on what a client may declare. They are sanity limits of our own, far
// outside anything a platform accepts, so a nonsense value is refused here.
const (
	maxDeclaredMS     = 24 * 60 * 60 * 1000
	maxDeclaredPixels = 100_000
)

func (f VideoFacts) check() error {
	switch {
	case f.DurationMS < 0 || f.DurationMS > maxDeclaredMS:
		return errors.New("duration_ms is out of range")
	case f.Width < 0 || f.Width > maxDeclaredPixels || f.Height < 0 || f.Height > maxDeclaredPixels:
		return errors.New("width or height is out of range")
	case (f.Width == 0) != (f.Height == 0):
		return errors.New("width and height go together")
	}
	return nil
}

// ReserveUpload records a pending upload and returns where to PUT it.
func (s *Service) ReserveUpload(ctx context.Context, userID uuid.UUID, contentType string, size int64, facts VideoFacts) (Upload, error) {
	if err := s.registry.AcceptsUpload(contentType, size); err != nil {
		return Upload{}, &BadRequest{Message: err.Error()}
	}
	if err := facts.check(); err != nil {
		return Upload{}, &BadRequest{Message: err.Error()}
	}
	if !publisher.IsVideo(contentType) {
		facts = VideoFacts{} // only video rules read them
	}

	id := uuid.New()
	// Keys are namespaced by user, so a bucket listing or a lifecycle rule
	// can be scoped to one person's files.
	key := uploadPrefix(userID) + media.Filename(id, contentType)

	if err := s.q.CreateMedia(ctx, db.CreateMediaParams{
		ID:          id,
		UserID:      userID,
		ObjectKey:   key,
		ContentType: contentType,
		SizeBytes:   size,
		DurationMs:  positive(facts.DurationMS),
		Width:       positive(facts.Width),
		Height:      positive(facts.Height),
	}); err != nil {
		return Upload{}, fmt.Errorf("reserve upload: %w", err)
	}

	url, err := s.storage.PresignUpload(ctx, key, uploadTTL)
	if err != nil {
		return Upload{}, err
	}
	return Upload{MediaID: id, URL: url, ContentType: contentType, ExpiresAt: s.now().Add(uploadTTL)}, nil
}

// uploadPrefix is where a user's uploads live in the bucket. Every object a
// user owns is under it, which is what lets DeleteUser remove them all.
func uploadPrefix(userID uuid.UUID) string {
	return "uploads/" + userID.String() + "/"
}

// positive stores zero, "not known", as NULL.
func positive(n int) *int32 {
	if n <= 0 {
		return nil
	}
	v := int32(n)
	return &v
}
