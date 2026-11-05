// Package instrument defines optional instrumentation hooks without a telemetry vendor dependency.
package instrument

import "context"

// Hook starts one bounded-cardinality operation. Done must be concurrency safe and called exactly once.
// Implementations must not panic. Operations contain SDK-defined names, never SQL or payloads.
type Hook func(context.Context, string) (context.Context, func(error))

// Start starts registered work with the supplied context.
func Start(ctx context.Context, h Hook, operation string) (context.Context, func(error)) {
	if h == nil {
		return ctx, func(error) {}
	}
	return h(ctx, operation)
}
