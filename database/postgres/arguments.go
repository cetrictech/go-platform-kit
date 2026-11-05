package postgres

import "github.com/jackc/pgx/v5"

// NamedArgs explicitly opts into pgx's SQL-aware @name parameter binding. No SQL literals or comments are rewritten by the SDK.
// Do not mutate this map while an operation using it is running.
type NamedArgs map[string]any

func bindArgs(args []any) []any {
	var out []any
	for i, v := range args {
		if n, ok := v.(NamedArgs); ok {
			if out == nil {
				out = append([]any(nil), args...)
			}
			out[i] = pgx.StrictNamedArgs(n)
		}
	}
	if out != nil {
		return out
	}
	return args
}
