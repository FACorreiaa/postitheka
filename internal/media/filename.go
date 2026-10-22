package media

import "github.com/google/uuid"

// extensions is spelled out rather than taken from mime.ExtensionsByType,
// whose answer depends on the host's mime database: image/jpeg can come back
// as .jfif on one machine and .jpg on another.
var extensions = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/heic":      ".heic",
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
}

// Filename names a file for a platform. Platforms decide how to render an
// attachment from its extension, so the extension has to match the type.
func Filename(id uuid.UUID, contentType string) string {
	ext, ok := extensions[contentType]
	if !ok {
		ext = ".bin"
	}
	return id.String() + ext
}
