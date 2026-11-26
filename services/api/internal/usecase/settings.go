package usecase

import (
	"errors"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Settings are business values read from configuration, never hard-coded in
// use cases. The composition root fills them from the environment;
// platform.Config will read the variables named on each field (not wired yet).
type Settings struct {
	// InvitationTTL mirrors the lifetime of the action link in the realm and
	// is shown as the invitation expiry.
	// Env: API_INVITATION_TTL (duration, e.g. 72h).
	InvitationTTL time.Duration
	// TrialDuration is the evaluation window of a new trial company.
	// Env: API_TRIAL_DURATION (duration, e.g. 336h).
	TrialDuration time.Duration
	// LastAccessInterval limits how often last_access_at is written.
	// Env: API_LAST_ACCESS_INTERVAL (duration, 0 disables the limit).
	LastAccessInterval time.Duration
	// DefaultPageSize and MaxPageSize bound list pagination.
	// Env: API_PAGE_SIZE_DEFAULT and API_PAGE_SIZE_MAX (integers).
	DefaultPageSize int
	MaxPageSize     int
	// Areas bounds the size of farms and fields and the drawing tolerance.
	// Env: API_FARM_MIN_AREA_HA, API_FIELD_MIN_AREA_HA and
	// API_FIELD_GEOMETRY_TOLERANCE_M2 (decimals).
	Areas domain.AreaRules
	// MaxDocumentBytes is the upload limit of an owner document.
	// Env: API_OWNER_DOCUMENT_MAX_BYTES (integer, default 10 MB).
	MaxDocumentBytes int64
}

// Validate rejects settings that would break use cases.
func (s Settings) Validate() error {
	switch {
	case s.InvitationTTL <= 0:
		return errors.New("invitation TTL must be positive")
	case s.TrialDuration <= 0:
		return errors.New("trial duration must be positive")
	case s.LastAccessInterval < 0:
		return errors.New("last access interval must not be negative")
	case s.DefaultPageSize <= 0 || s.MaxPageSize < s.DefaultPageSize:
		return errors.New("page sizes must satisfy 0 < default <= max")
	case s.Areas.MinFarmAreaHa <= 0 || s.Areas.MinFieldAreaHa <= 0:
		return errors.New("minimum farm and field areas must be positive")
	case s.Areas.ToleranceM2 < 0:
		return errors.New("field geometry tolerance must not be negative")
	case s.MaxDocumentBytes <= 0:
		return errors.New("owner document size limit must be positive")
	}
	return nil
}

// pageRequest validates a client page. Zero values mean "use the default".
func (s Settings) pageRequest(page, pageSize int) (PageRequest, error) {
	var problems domain.Violations
	if page == 0 {
		page = 1
	}
	if page < 1 {
		problems.Add("page", "a página começa em 1")
	}
	if pageSize == 0 {
		pageSize = s.DefaultPageSize
	}
	if pageSize < 1 || pageSize > s.MaxPageSize {
		problems.Add("page_size", "tamanho de página fora do limite")
	}
	if err := problems.Err(); err != nil {
		return PageRequest{}, err
	}
	return PageRequest{Page: page, PageSize: pageSize}, nil
}
