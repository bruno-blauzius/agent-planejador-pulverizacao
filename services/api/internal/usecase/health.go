package usecase

import (
	"context"
	"log/slog"
)

// Status is the outcome of a health check.
type Status string

// Possible outcomes of a health check.
const (
	StatusUp   Status = "up"
	StatusDown Status = "down"
)

// CheckResult is the outcome of one named check inside a report.
type CheckResult struct {
	Name   string
	Status Status
}

// HealthReport is the compact outcome of a probe.
type HealthReport struct {
	Status Status
	Checks []CheckResult
}

// Healthy reports whether the overall status is up.
func (r HealthReport) Healthy() bool { return r.Status == StatusUp }

// DependencyProbe verifies one external dependency (database, agent service...).
// Implementations live in adapters and are registered in the composition root.
type DependencyProbe interface {
	Name() string
	Check(ctx context.Context) error
}

// CheckLiveness answers "is the process alive?". It never inspects
// dependencies: restarting the pod would not fix a failing database.
type CheckLiveness struct{}

// Execute always reports up while the process can answer.
func (CheckLiveness) Execute() HealthReport {
	return HealthReport{Status: StatusUp}
}

// CheckStartup answers "has initialization finished?".
type CheckStartup struct {
	lifecycle *Lifecycle
}

// NewCheckStartup builds the startup check.
func NewCheckStartup(lifecycle *Lifecycle) *CheckStartup {
	return &CheckStartup{lifecycle: lifecycle}
}

// Execute reports up once the lifecycle was marked as started.
func (c *CheckStartup) Execute() HealthReport {
	if c.lifecycle.IsStarted() {
		return HealthReport{Status: StatusUp}
	}
	return HealthReport{Status: StatusDown}
}

// CheckReadiness answers "can this instance receive traffic right now?".
type CheckReadiness struct {
	lifecycle *Lifecycle
	probes    []DependencyProbe
	logger    *slog.Logger
}

// NewCheckReadiness builds the readiness check over the given dependency probes.
func NewCheckReadiness(lifecycle *Lifecycle, logger *slog.Logger, probes ...DependencyProbe) *CheckReadiness {
	return &CheckReadiness{lifecycle: lifecycle, probes: probes, logger: logger}
}

// Execute reports down while starting, while draining, or when any dependency fails.
func (c *CheckReadiness) Execute(ctx context.Context) HealthReport {
	report := HealthReport{Status: StatusUp}
	if !c.lifecycle.IsStarted() || c.lifecycle.IsShuttingDown() {
		report.Status = StatusDown
		return report
	}

	for _, probe := range c.probes {
		result := CheckResult{Name: probe.Name(), Status: StatusUp}
		if err := probe.Check(ctx); err != nil {
			// The cause is logged, never returned: probe bodies are reachable over the network.
			c.logger.WarnContext(ctx, "dependency check failed", "dependency", probe.Name(), "error", err)
			result.Status = StatusDown
			report.Status = StatusDown
		}
		report.Checks = append(report.Checks, result)
	}
	return report
}
