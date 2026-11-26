package domain

import "regexp"

// Municipality is a Brazilian municipality of the IBGE registry.
type Municipality struct {
	// Code is the 7-digit IBGE code.
	Code  string
	Name  string
	State string
}

var municipalityCodePattern = regexp.MustCompile(`^[0-9]{7}$`)

// NewMunicipality validates an entry of the IBGE registry.
func NewMunicipality(code, name, state string) (Municipality, error) {
	var problems Violations
	if !municipalityCodePattern.MatchString(code) {
		problems.Add("code", "código IBGE deve ter 7 dígitos")
	}
	trimmed, err := ParseName("name", name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return Municipality{}, mergeErr
	}
	normalized := normalizeState(state)
	if !validState(normalized) {
		problems.Add("state", "UF deve ter 2 letras")
	}
	if err := problems.Err(); err != nil {
		return Municipality{}, err
	}
	return Municipality{Code: code, Name: trimmed, State: normalized}, nil
}
