package domain

import (
	"fmt"
	"strings"
	"time"
)

// CompanyPlan is the commercial plan of a company.
type CompanyPlan string

// Company plans.
const (
	PlanBasic CompanyPlan = "basic"
	PlanPro   CompanyPlan = "pro"
	PlanTrial CompanyPlan = "trial"
)

// Valid reports whether p is a known plan.
func (p CompanyPlan) Valid() bool {
	return p == PlanBasic || p == PlanPro || p == PlanTrial
}

// CompanyStatus is the lifecycle status of a company. An inactive company
// cuts the access of all its users (ADR 0013).
type CompanyStatus string

// Company statuses.
const (
	CompanyActive   CompanyStatus = "active"
	CompanyTrial    CompanyStatus = "trial"
	CompanyInactive CompanyStatus = "inactive"
)

// Valid reports whether s is a known status.
func (s CompanyStatus) Valid() bool {
	return s == CompanyActive || s == CompanyTrial || s == CompanyInactive
}

// Company is the tenant: owner of its farms, fields, owners, users and applications.
type Company struct {
	ID                 CompanyID
	Name               string
	Document           Document
	City               string
	State              string
	Plan               CompanyPlan
	Status             CompanyStatus
	TrialEndsAt        *time.Time
	IsPlatformOperator bool
	CreatedAt          time.Time
	DeletedAt          *time.Time
	Version            int
}

// NewCompanyInput carries the raw fields to register a client company.
type NewCompanyInput struct {
	Name     string
	Document string
	City     string
	State    string
	Plan     CompanyPlan
}

// NewCompany validates the input and returns a client company (never the
// platform operator). The trial plan starts the evaluation window.
func NewCompany(id CompanyID, in NewCompanyInput, now time.Time, trialDuration time.Duration) (Company, error) {
	var problems Violations
	name, err := ParseName("name", in.Name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return Company{}, mergeErr
	}
	document, err := ParseDocument(in.Document)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return Company{}, mergeErr
	}
	city, state := strings.TrimSpace(in.City), normalizeState(in.State)
	if city == "" {
		problems.Add("city", "informe a cidade")
	}
	if !validState(state) {
		problems.Add("state", "UF deve ter 2 letras")
	}
	if !in.Plan.Valid() {
		problems.Add("plan", fmt.Sprintf("plano desconhecido: %q", in.Plan))
	}
	if err := problems.Err(); err != nil {
		return Company{}, err
	}

	company := Company{
		ID:        id,
		Name:      name,
		Document:  document,
		City:      city,
		State:     state,
		Plan:      in.Plan,
		Status:    CompanyActive,
		CreatedAt: now,
	}
	if in.Plan == PlanTrial {
		company.startTrial(now, trialDuration)
	}
	return company, nil
}

// AllowsAccess reports whether the company's users may use the product.
func (c Company) AllowsAccess() bool {
	return c.DeletedAt == nil && c.Status != CompanyInactive
}

// UpdateDetails changes the descriptive fields that are non-nil.
func (c *Company) UpdateDetails(name, city, state *string) error {
	var problems Violations
	updated := *c
	if name != nil {
		parsed, err := ParseName("name", *name)
		if mergeErr := problems.Merge(err); mergeErr != nil {
			return mergeErr
		}
		updated.Name = parsed
	}
	if city != nil {
		updated.City = strings.TrimSpace(*city)
		if updated.City == "" {
			problems.Add("city", "informe a cidade")
		}
	}
	if state != nil {
		updated.State = normalizeState(*state)
		if !validState(updated.State) {
			problems.Add("state", "UF deve ter 2 letras")
		}
	}
	if err := problems.Err(); err != nil {
		return err
	}
	*c = updated
	return nil
}

// ChangePlan switches the plan. Moving to trial starts the evaluation window;
// leaving trial ends it and turns an evaluating company into an active one.
func (c *Company) ChangePlan(plan CompanyPlan, now time.Time, trialDuration time.Duration) error {
	if !plan.Valid() {
		return Invalid("plan", "plano desconhecido: %q", plan)
	}
	c.Plan = plan
	switch {
	case plan == PlanTrial && c.Status != CompanyInactive:
		c.Status = CompanyTrial
		if c.TrialEndsAt == nil {
			c.startTrial(now, trialDuration)
		}
	case plan != PlanTrial && c.Status == CompanyTrial:
		c.Status = CompanyActive
		c.TrialEndsAt = nil
	}
	return nil
}

// ChangeStatus sets the status. Evaluation (trial) is only coherent with the
// trial plan and always carries an end date.
func (c *Company) ChangeStatus(status CompanyStatus, now time.Time, trialDuration time.Duration) error {
	if !status.Valid() {
		return Invalid("status", "status desconhecido: %q", status)
	}
	if status == CompanyTrial {
		if c.Plan != PlanTrial {
			return Invalid("status", "avaliação exige o plano trial")
		}
		if c.TrialEndsAt == nil {
			c.startTrial(now, trialDuration)
		}
	}
	c.Status = status
	return nil
}

// MarkDeleted soft deletes the company. A deleted company is always inactive
// (ck_companies_deleted), whatever status it had.
func (c *Company) MarkDeleted(now time.Time) {
	c.Status = CompanyInactive
	c.DeletedAt = &now
}

func (c *Company) startTrial(now time.Time, duration time.Duration) {
	end := now.Add(duration)
	c.Status = CompanyTrial
	c.TrialEndsAt = &end
}

func normalizeState(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validState(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
