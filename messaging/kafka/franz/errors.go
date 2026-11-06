package franz

import (
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/twmb/franz-go/pkg/kerr"
)

// Classify maps broker error codes without exposing broker diagnostics to customers.
func Classify(e error) error {
	if e == nil {
		return nil
	}
	var k *kerr.Error
	if errors.As(e, &k) {
		return app.Wrap(e, "Message delivery could not be completed.", app.WithCategory(app.Messaging), app.WithCode("KAFKA_ERROR"), app.WithRetryable(k.Retriable), app.WithHTTPStatus(503))
	}
	return app.Classify(e)
}
