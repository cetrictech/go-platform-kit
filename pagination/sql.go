package pagination

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Sort names refer to an explicit allowlist. The final column must be unique for deterministic paging.
type Sort struct {
	Name                           string
	Descending, NullsFirst, Unique bool
}

// SQLPage contains bound seek SQL and its required result ordering.
type SQLPage struct {
	Where, OrderBy string
	Args           []any
	Reverse        bool
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// Keyset builds a lexicographic seek predicate. Values are in Sort order; nil represents SQL NULL.
// PostgreSQL placeholders start at start+1. MySQL uses ?. Previous pages must reverse result rows.
func Keyset(dialect string, allow map[string]string, sorts []Sort, values []any, previous bool, start int) (SQLPage, error) {
	out := SQLPage{Reverse: previous}
	if dialect != "postgres" && dialect != "mysql" {
		return out, errors.New("pagination: unsupported dialect")
	}
	if start < 0 || len(sorts) == 0 || !sorts[len(sorts)-1].Unique || len(values) != 0 && len(values) != len(sorts) {
		return out, errors.New("pagination: invalid sort or key arity")
	}
	quote := "\""
	if dialect == "mysql" {
		quote = "`"
	}
	columns := make([]string, len(sorts))
	orders := make([]string, len(sorts))
	for i, s := range sorts {
		c, ok := allow[s.Name]
		if !ok || !identifier.MatchString(c) {
			return out, errors.New("pagination: column not allowed")
		}
		parts := strings.Split(c, ".")
		for j := range parts {
			parts[j] = quote + parts[j] + quote
		}
		columns[i] = strings.Join(parts, ".")
		desc, nf := s.Descending, s.NullsFirst
		if previous {
			desc, nf = !desc, !nf
		}
		dir := "ASC"
		if desc {
			dir = "DESC"
		}
		if dialect == "postgres" {
			nulls := "NULLS LAST"
			if nf {
				nulls = "NULLS FIRST"
			}
			orders[i] = columns[i] + " " + dir + " " + nulls
		} else {
			nullDir := "ASC"
			if nf {
				nullDir = "DESC"
			}
			orders[i] = "(" + columns[i] + " IS NULL) " + nullDir + ", " + columns[i] + " " + dir
		}
	}
	out.OrderBy = strings.Join(orders, ", ")
	if len(values) == 0 {
		return out, nil
	}
	bind := func(v any) string {
		out.Args = append(out.Args, v)
		if dialect == "postgres" {
			return fmt.Sprintf("$%d", start+len(out.Args))
		}
		return "?"
	}
	var branches []string
	for i, s := range sorts {
		var parts []string
		for j := 0; j < i; j++ {
			if values[j] == nil {
				parts = append(parts, columns[j]+" IS NULL")
			} else {
				parts = append(parts, columns[j]+" = "+bind(values[j]))
			}
		}
		desc, nf := s.Descending, s.NullsFirst
		if previous {
			desc, nf = !desc, !nf
		}
		c := columns[i]
		var cmp string
		if values[i] == nil {
			if nf {
				cmp = c + " IS NOT NULL"
			} else {
				cmp = "FALSE"
			}
		} else {
			op := ">"
			if desc {
				op = "<"
			}
			cmp = c + " " + op + " " + bind(values[i])
			if !nf {
				cmp = "(" + cmp + " OR " + c + " IS NULL)"
			}
		}
		parts = append(parts, cmp)
		branches = append(branches, "("+strings.Join(parts, " AND ")+")")
	}
	out.Where = "(" + strings.Join(branches, " OR ") + ")"
	return out, nil
}
