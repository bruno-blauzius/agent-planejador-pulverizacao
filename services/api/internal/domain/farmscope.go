package domain

import "slices"

// ScopeMode says how a user's farms are determined.
type ScopeMode string

// Scope modes.
const (
	ScopeAll      ScopeMode = "all"
	ScopeSelected ScopeMode = "selected"
)

// FarmScope is the set of farms a user sees: every farm of the company or an
// explicit non-empty list. The zero value means "all".
type FarmScope struct {
	mode    ScopeMode
	farmIDs []FarmID
}

// AllFarms returns the scope that covers every farm of the company.
func AllFarms() FarmScope { return FarmScope{mode: ScopeAll} }

// NewSelectedFarms returns a scope limited to the given farms (non-empty, no duplicates).
func NewSelectedFarms(ids []FarmID) (FarmScope, error) {
	if len(ids) == 0 {
		return FarmScope{}, Invalid("farm_scope.farm_ids", "selecione ao menos uma fazenda")
	}
	seen := make(map[FarmID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return FarmScope{}, Invalid("farm_scope.farm_ids", "identificador de fazenda vazio")
		}
		if _, dup := seen[id]; dup {
			return FarmScope{}, Invalid("farm_scope.farm_ids", "fazenda repetida")
		}
		seen[id] = struct{}{}
	}
	return FarmScope{mode: ScopeSelected, farmIDs: slices.Clone(ids)}, nil
}

// ParseFarmScope builds a scope from the API representation.
func ParseFarmScope(mode string, ids []string) (FarmScope, error) {
	switch ScopeMode(mode) {
	case ScopeAll:
		if len(ids) > 0 {
			return FarmScope{}, Invalid("farm_scope.farm_ids", "não informe fazendas com o modo all")
		}
		return AllFarms(), nil
	case ScopeSelected:
		farmIDs := make([]FarmID, len(ids))
		for i, id := range ids {
			farmIDs[i] = FarmID(id)
		}
		return NewSelectedFarms(farmIDs)
	default:
		return FarmScope{}, Invalid("farm_scope.mode", "modo de escopo desconhecido: %q", mode)
	}
}

// Mode returns the scope mode ("all" for the zero value).
func (s FarmScope) Mode() ScopeMode {
	if s.mode == "" {
		return ScopeAll
	}
	return s.mode
}

// FarmIDs returns a copy of the selected farms (empty for "all").
func (s FarmScope) FarmIDs() []FarmID { return slices.Clone(s.farmIDs) }

// IsSelected reports whether the scope lists specific farms.
func (s FarmScope) IsSelected() bool { return s.Mode() == ScopeSelected }

// Allows reports whether the scope includes the farm.
func (s FarmScope) Allows(id FarmID) bool {
	return !s.IsSelected() || slices.Contains(s.farmIDs, id)
}

// Equal compares two scopes ignoring the order of the selected farms.
func (s FarmScope) Equal(other FarmScope) bool {
	if s.Mode() != other.Mode() || len(s.farmIDs) != len(other.farmIDs) {
		return false
	}
	a, b := slices.Clone(s.farmIDs), slices.Clone(other.farmIDs)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
