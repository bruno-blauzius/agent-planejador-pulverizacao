package fakes

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// SoilHistory is an in-memory soil_state_history: it implements
// usecase.SoilHistoryStore over the fields of a Registry, with the UNIQUE
// (company, field, local day) of the table. It is safe for the concurrent use
// of the executor.
type SoilHistory struct {
	reg *Registry
	// Companies are the active companies the executor walks.
	Companies []domain.CompanyID
	// FailRecord makes RecordSoilState fail for one field.
	FailRecord map[domain.FieldID]error
	// FailFields makes ListSoilHistoryFields fail for one company.
	FailFields map[domain.CompanyID]error
	// FailCompanies makes ListSoilHistoryCompanies fail.
	FailCompanies error

	mu      sync.Mutex
	records map[historyKey]usecase.SoilStateRecord
}

type historyKey struct {
	company domain.CompanyID
	field   domain.FieldID
	day     string
}

// NewSoilHistory returns an empty history over the registry fields.
func NewSoilHistory(reg *Registry, companies ...domain.CompanyID) *SoilHistory {
	return &SoilHistory{
		reg: reg, Companies: companies,
		FailRecord: map[domain.FieldID]error{}, FailFields: map[domain.CompanyID]error{},
		records: map[historyKey]usecase.SoilStateRecord{},
	}
}

func keyOf(companyID domain.CompanyID, fieldID domain.FieldID, day time.Time) historyKey {
	return historyKey{company: companyID, field: fieldID, day: day.Format(time.DateOnly)}
}

// Records returns every stored record, by company, field and day.
func (h *SoilHistory) Records() []usecase.SoilStateRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	records := make([]usecase.SoilStateRecord, 0, len(h.records))
	for _, record := range h.records {
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b usecase.SoilStateRecord) int {
		return cmp.Or(cmp.Compare(a.Field.CompanyID, b.Field.CompanyID), cmp.Compare(a.Field.FieldID, b.Field.FieldID), a.LocalDay.Compare(b.LocalDay))
	})
	return records
}

// ListSoilHistoryCompanies implements usecase.SoilHistoryStore.
func (h *SoilHistory) ListSoilHistoryCompanies(context.Context) ([]domain.CompanyID, error) {
	if h.FailCompanies != nil {
		return nil, h.FailCompanies
	}
	return slices.Clone(h.Companies), nil
}

// ListSoilHistoryFields implements usecase.SoilHistoryStore.
func (h *SoilHistory) ListSoilHistoryFields(_ context.Context, companyID domain.CompanyID, day time.Time) ([]usecase.SoilHistoryField, error) {
	if err := h.FailFields[companyID]; err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	fields := []usecase.SoilHistoryField{}
	for _, field := range h.reg.Fields {
		if field.CompanyID != companyID {
			continue
		}
		_, recorded := h.records[keyOf(companyID, field.ID, day)]
		fields = append(fields, usecase.SoilHistoryField{
			Field:    usecase.FieldRef{CompanyID: companyID, FarmID: field.FarmID, FieldID: field.ID},
			Recorded: recorded,
		})
	}
	slices.SortFunc(fields, func(a, b usecase.SoilHistoryField) int { return cmp.Compare(a.Field.FieldID, b.Field.FieldID) })
	return fields, nil
}

// RecordSoilState implements usecase.SoilHistoryStore (ON CONFLICT DO NOTHING).
func (h *SoilHistory) RecordSoilState(_ context.Context, record usecase.SoilStateRecord) (bool, error) {
	if err := h.FailRecord[record.Field.FieldID]; err != nil {
		return false, fmt.Errorf("record soil state: %w", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	key := keyOf(record.Field.CompanyID, record.Field.FieldID, record.LocalDay)
	if _, taken := h.records[key]; taken {
		return false, nil
	}
	h.records[key] = record
	return true, nil
}
