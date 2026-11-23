package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func TestValidUUID(t *testing.T) {
	cases := map[string]bool{
		"123e4567-e89b-12d3-a456-426614174000":  true,
		"123E4567-E89B-12D3-A456-426614174000":  true,
		"":                                      false,
		"not-a-uuid":                            false,
		"123e4567e89b12d3a456426614174000":      false,
		"123e4567-e89b-12d3-a456-42661417400g":  false,
		"123e4567-e89b-12d3-a456-4266141740000": false,
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := validUUID(input); got != want {
				t.Fatalf("validUUID(%q) = %v, want %v", input, got, want)
			}
		})
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ana", "%ana%"},
		{"  ana  ", "%ana%"},
		{"100%", `%100\%%`},
		{"a_b", `%a\_b%`},
		{"c:\\dir", "%c:\\\\dir%"},
	}
	for _, c := range cases {
		got := likePattern(c.in)
		if !got.Valid || got.String != c.want {
			t.Errorf("likePattern(%q) = %+v, want %q", c.in, got, c.want)
		}
	}
	if likePattern("   ").Valid || likePattern("").Valid {
		t.Error("blank search must mean no filter")
	}
}

func TestPageBounds(t *testing.T) {
	cases := []struct {
		page, size    int
		limit, offset int32
	}{
		{1, 20, 20, 0},
		{3, 20, 20, 40},
		{0, 0, 1, 0},
		{1 << 40, 1 << 40, 1<<31 - 1, 1<<31 - 1},
	}
	for _, c := range cases {
		limit, offset := pageBounds(usecase.PageRequest{Page: c.page, PageSize: c.size})
		if limit != c.limit || offset != c.offset {
			t.Errorf("pageBounds(%d, %d) = %d, %d; want %d, %d", c.page, c.size, limit, offset, c.limit, c.offset)
		}
	}
}

func TestScopeCompany(t *testing.T) {
	if company, ok := scopeCompany(usecase.Scope{}); ok || company != nil {
		t.Error("an empty scope matches nothing")
	}
	id := "123e4567-e89b-12d3-a456-426614174000"
	if company, ok := scopeCompany(usecase.OnlyCompany("123e4567-e89b-12d3-a456-426614174000")); !ok || company == nil || *company != id {
		t.Error("a company scope filters by that company")
	}
	if _, ok := scopeCompany(usecase.OnlyCompany("nope")); ok {
		t.Error("a malformed company id matches nothing")
	}
}

func TestTranslateWriteMapsConstraintsToPortErrors(t *testing.T) {
	pg := func(code, constraint string) error {
		return &pgconn.PgError{Code: code, ConstraintName: constraint}
	}
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"email", pg(sqlStateUnique, "uq_users_email_active"), usecase.ErrEmailAlreadyRegistered},
		{"cnpj", pg(sqlStateUnique, "uq_companies_document"), usecase.ErrDocumentAlreadyRegistered},
		{"identity", pg(sqlStateUnique, "uq_user_identities_issuer_subject"), usecase.ErrIdentityAlreadyLinked},
		{"farm", pg(sqlStateForeignKey, "fk_user_farm_access_farm"), usecase.ErrFarmNotInCompany},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := translateWrite("op", c.err); !errors.Is(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
	var integrity *IntegrityError
	if got := translateWrite("op", pg(sqlStateCheck, "ck_users_role")); !errors.As(got, &integrity) || integrity.Kind != "check" {
		t.Fatalf("an unmapped check violation must be an IntegrityError, got %v", got)
	}
	if got := translateWrite("op", nil); got != nil {
		t.Fatal("nil stays nil")
	}
	plain := errors.New("connection reset")
	if got := translateWrite("op", plain); !errors.Is(got, plain) {
		t.Fatal("other errors keep their cause")
	}
}
