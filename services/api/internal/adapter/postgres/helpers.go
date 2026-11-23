package postgres

import (
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// scopeCompany turns a Scope into the nullable company filter of the queries:
// nil means every company (super admin). ok is false when the scope can match
// nothing (no company, or an id that is not a UUID), so callers answer "empty"
// without asking the database.
func scopeCompany(scope usecase.Scope) (company *string, ok bool) {
	if scope.All() {
		return nil, true
	}
	id := string(scope.CompanyID())
	if !validUUID(id) {
		return nil, false
	}
	return &id, true
}

// optionalCompany is the extra company filter of a listing; empty means none.
func optionalCompany(id string) (company *string, ok bool) {
	if id == "" {
		return nil, true
	}
	if !validUUID(id) {
		return nil, false
	}
	return &id, true
}

// validUUID reports whether s has the canonical 8-4-4-4-12 hex form.
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if s[i] != '-' {
				return false
			}
		case !isHex(s[i]):
			return false
		}
	}
	return true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// likePattern builds a case-insensitive "contains" pattern for ILIKE ... ESCAPE '\'.
// The user's text is escaped, so % and _ match themselves. Blank search means no filter.
func likePattern(search string) pgtype.Text {
	search = strings.TrimSpace(search)
	if search == "" {
		return pgtype.Text{}
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
	return pgtype.Text{String: "%" + escaped + "%", Valid: true}
}

// optionalText is a nullable text filter; empty means none.
func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

// pageBounds converts a validated 1-based page into LIMIT/OFFSET.
func pageBounds(page usecase.PageRequest) (limit, offset int32) {
	size := int64(max(page.PageSize, 1))
	number := int64(max(page.Page, 1))
	if number-1 > math.MaxInt32/size {
		return clampInt32(size), math.MaxInt32 // beyond any real table: an empty page
	}
	return clampInt32(size), clampInt32((number - 1) * size)
}

// normalizeVersion treats the zero version of a freshly built entity as the
// first stored version (the column starts at 1).
func normalizeVersion(version int) int32 {
	return clampInt32(int64(max(version, 1)))
}

func clampInt32(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < math.MinInt32 {
		return math.MinInt32
	}
	return int32(value)
}
