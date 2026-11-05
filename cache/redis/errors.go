package redis

import (
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	r "github.com/redis/go-redis/v9"
)

// Classify maps Redis misses and optimistic transaction conflicts while preserving errors.Is behavior.
func Classify(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, r.Nil) {
		return app.Wrap(e, "The requested value was not found.", app.WithCode("CACHE_MISS"), app.WithCategory(app.NotFound), app.WithHTTPStatus(404))
	}
	if errors.Is(e, r.TxFailedErr) {
		return app.Wrap(e, "The value changed. Please try again.", app.WithCode("CACHE_CONFLICT"), app.WithCategory(app.Conflict), app.WithHTTPStatus(409), app.WithRetryable(true))
	}
	return app.Classify(e)
}
