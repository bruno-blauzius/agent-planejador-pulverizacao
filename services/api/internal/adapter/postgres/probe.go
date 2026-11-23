package postgres

import (
	"context"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Probe is the readiness check of the database (usecase.DependencyProbe).
// Liveness never uses it: restarting the pod would not fix a down database.
type Probe struct {
	db interface {
		Ping(ctx context.Context) error
	}
}

// NewProbe builds the readiness probe over the database.
func NewProbe(db *Database) *Probe { return &Probe{db: db} }

// Name implements usecase.DependencyProbe.
func (*Probe) Name() string { return "postgres" }

// Check implements usecase.DependencyProbe. The caller bounds ctx with the probe timeout.
func (p *Probe) Check(ctx context.Context) error { return p.db.Ping(ctx) }

var _ usecase.DependencyProbe = (*Probe)(nil)
