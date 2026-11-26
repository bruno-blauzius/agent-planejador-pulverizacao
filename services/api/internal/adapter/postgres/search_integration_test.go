//go:build integration

package postgres_test

import (
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The registry search ignores accents and case (unaccent + ILIKE, ADR 0017).
func TestRegistrySearchIgnoresAccents(t *testing.T) {
	company := newCompany(t)
	farm := farmValue(company, "Fazenda São João")
	insertFarm(t, farm)
	newOwner(t, company, "Conceição Araújo")
	page := usecase.PageRequest{Page: 1, PageSize: 20}

	tests := []struct {
		name, search string
		wantFarms    int
		wantOwners   int
	}{
		{"without accents", "sao joao", 1, 0},
		{"with accents", "SÃO JOÃO", 1, 0},
		{"city without accent", "guaxupe", 1, 0},
		{"owner without accents", "conceicao arau", 0, 1},
		{"no match", "esperanca", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			farms, err := testDB.ListFarms(ctx(), company.ID, usecase.FarmFilter{Search: tt.search, Visible: domain.AllFarms()}, page)
			must(t, err)
			owners, err := testDB.ListOwners(ctx(), company.ID, usecase.OwnerFilter{Search: tt.search, Visible: domain.AllFarms()}, page)
			must(t, err)
			if farms.Total != tt.wantFarms || owners.Total != tt.wantOwners {
				t.Fatalf("farms = %d, owners = %d; want %d and %d", farms.Total, owners.Total, tt.wantFarms, tt.wantOwners)
			}
		})
	}
}
