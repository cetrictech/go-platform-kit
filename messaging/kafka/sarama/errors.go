package sarama

import (
	"errors"
	s "github.com/IBM/sarama"
	app "github.com/cetrictech/go-platform-kit/errors"
)

// Classify maps common transient broker codes and preserves the original error chain.
func Classify(e error) error {
	if e == nil {
		return nil
	}
	var k s.KError
	if errors.As(e, &k) {
		retry := k == s.ErrBrokerNotAvailable || k == s.ErrLeaderNotAvailable || k == s.ErrNotLeaderForPartition || k == s.ErrRequestTimedOut || k == s.ErrNotEnoughReplicas || k == s.ErrNotEnoughReplicasAfterAppend
		return app.Wrap(e, "Message delivery could not be completed.", app.WithCategory(app.Messaging), app.WithCode("KAFKA_ERROR"), app.WithRetryable(retry), app.WithHTTPStatus(503))
	}
	return app.Classify(e)
}
