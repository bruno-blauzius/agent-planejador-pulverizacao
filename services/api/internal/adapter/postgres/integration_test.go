//go:build integration

// Integration tests of the Postgres adapter. They need a database with the
// migrations applied and the runtime role: run them with
// `make api-test-integration`, which creates and removes a throwaway container.
// The connection comes from API_TEST_DATABASE_URL and must use api_app, so the
// tests exercise the same privileges as production (DML only).
package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const testTouchInterval = time.Hour

var (
	testDB   *postgres.Database
	testPool *pgxpool.Pool

	operatorOnce    sync.Once
	operatorCompany domain.Company
)

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	url := os.Getenv("API_TEST_DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "API_TEST_DATABASE_URL is not set: run `make api-test-integration`")
		return 1
	}
	cfg, err := platform.LoadDatabaseConfig(func(key string) (string, bool) {
		if key == "API_DATABASE_URL" {
			return url, true
		}
		return "", false
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "database config:", err)
		return 1
	}
	cfg.MaxConns = 20 // concurrency tests hold several transactions at once
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database pool:", err)
		return 1
	}
	defer pool.Close()
	testPool = pool
	testDB = postgres.NewDatabase(pool, postgres.Options{SessionTouchInterval: testTouchInterval})
	return m.Run()
}

// ---- fixtures ------------------------------------------------------------------------

func ctx() context.Context { return context.Background() }

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func randomDigits(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i := range buf {
		buf[i] = '0' + buf[i]%10
	}
	return string(buf)
}

func newCompany(t *testing.T, mods ...func(*domain.Company)) domain.Company {
	t.Helper()
	id := newID()
	company := domain.Company{
		ID:        domain.CompanyID(id),
		Name:      "Empresa " + id[:8],
		Document:  domain.Document(randomDigits(14)),
		City:      "Uberaba",
		State:     "MG",
		Plan:      domain.PlanBasic,
		Status:    domain.CompanyActive,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	for _, mod := range mods {
		mod(&company)
	}
	must(t, testDB.InsertCompany(ctx(), company))
	return company
}

func operator(t *testing.T) domain.Company {
	t.Helper()
	operatorOnce.Do(func() {
		operatorCompany = newCompany(t, func(c *domain.Company) {
			c.Name = "Operadora " + string(c.ID)[:8]
			c.IsPlatformOperator = true
		})
	})
	return operatorCompany
}

func newUser(t *testing.T, company domain.Company, mods ...func(*domain.User)) domain.User {
	t.Helper()
	id := newID()
	user := domain.User{
		ID:        domain.UserID(id),
		CompanyID: company.ID,
		Name:      "Usuário " + id[:8],
		Email:     domain.Email(id + "@test.local"),
		Role:      domain.RoleAdmin,
		Status:    domain.UserActive,
		FarmScope: domain.AllFarms(),
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	for _, mod := range mods {
		mod(&user)
	}
	must(t, testDB.InsertUser(ctx(), user))
	return user
}

// newFarm inserts a farm with plain SQL: the farm adapter does not exist yet.
func newFarm(t *testing.T, company domain.Company) domain.FarmID {
	t.Helper()
	id := newID()
	_, err := testPool.Exec(ctx(),
		`INSERT INTO farms (id, company_id, name, production_type, city, state) VALUES ($1, $2, $3, 'grains', 'Uberaba', 'MG')`,
		id, string(company.ID), "Fazenda "+id[:8])
	must(t, err)
	return domain.FarmID(id)
}

func selected(t *testing.T, ids ...domain.FarmID) domain.FarmScope {
	t.Helper()
	scope, err := domain.NewSelectedFarms(ids)
	must(t, err)
	return scope
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func requireIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}

func inCompany(c domain.Company) usecase.Scope { return usecase.OnlyCompany(c.ID) }

func allCompanies() usecase.Scope {
	return usecase.ScopeOf(domain.Principal{Permissions: []domain.Permission{domain.PermUsersManageAll}})
}

func firstPage(size int) usecase.PageRequest { return usecase.PageRequest{Page: 1, PageSize: size} }
