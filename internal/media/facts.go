package media

import "time"

// Declared turns the nullable columns a client declared for a video into the
// values a publisher's rules read, zero where nothing was declared.
func Declared(durationMS, width, height *int32) (time.Duration, int, int) {
	var d time.Duration
	var w, h int
	if durationMS != nil {
		d = time.Duration(*durationMS) * time.Millisecond
	}
	if width != nil && height != nil {
		w, h = int(*width), int(*height)
	}
	return d, w, h
}
