package fakes

import (
	"context"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// ProviderAccount is an account of the fake realm.
type ProviderAccount struct {
	Account usecase.Account
	Name    string
	// Used is true when the person already signed in (the account cannot be deleted).
	Used bool
}

// SentEmail records an action e-mail requested from the provider.
type SentEmail struct {
	Subject string
	Kind    usecase.EmailKind
}

// Provider is an in-memory identity provider implementing AccountDirectory,
// ProductAccess and InviteSender. It never disables accounts: there is no
// such operation in the ports, which is what the tests rely on.
type Provider struct {
	Issuer   string
	Accounts map[string]*ProviderAccount // by subject
	Groups   map[string]domain.Role      // product group membership by subject
	// EndedSessions lists the subjects whose product sessions were ended, in order.
	EndedSessions []string
	Emails        []SentEmail
	// Calls is the ordered log of operations, for asserting the flow order.
	Calls []string

	failures map[string]error
	counter  int
}

// NewProvider returns an empty realm.
func NewProvider(issuer string) *Provider {
	return &Provider{
		Issuer:   issuer,
		Accounts: map[string]*ProviderAccount{},
		Groups:   map[string]domain.Role{},
		failures: map[string]error{},
	}
}

// Fail makes every call of the operation return err until Heal.
func (p *Provider) Fail(operation string, err error) { p.failures[operation] = err }

// Heal removes the injected failure.
func (p *Provider) Heal(operation string) { delete(p.failures, operation) }

func (p *Provider) enter(operation string) error {
	p.Calls = append(p.Calls, operation)
	if err := p.failures[operation]; err != nil {
		return fmt.Errorf("provider %s: %w", operation, err)
	}
	return nil
}

// SeedAccount creates an account that exists before the product knows it (a
// person who uses another Inceres product).
func (p *Provider) SeedAccount(email domain.Email, verified bool) usecase.Account {
	p.counter++
	account := usecase.Account{Issuer: p.Issuer, Subject: fmt.Sprintf("sub-%d", p.counter), Email: email, EmailVerified: verified}
	p.Accounts[account.Subject] = &ProviderAccount{Account: account, Used: verified}
	return account
}

// FindAccountByEmail implements usecase.AccountDirectory.
func (p *Provider) FindAccountByEmail(_ context.Context, email domain.Email) (usecase.Account, error) {
	if err := p.enter("FindAccountByEmail"); err != nil {
		return usecase.Account{}, err
	}
	for _, stored := range p.Accounts {
		if stored.Account.Email == email {
			return stored.Account, nil
		}
	}
	return usecase.Account{}, usecase.ErrAccountNotFound
}

// CreateAccount implements usecase.AccountDirectory.
func (p *Provider) CreateAccount(_ context.Context, in usecase.NewAccount) (usecase.Account, error) {
	if err := p.enter("CreateAccount"); err != nil {
		return usecase.Account{}, err
	}
	p.counter++
	account := usecase.Account{Issuer: p.Issuer, Subject: fmt.Sprintf("sub-%d", p.counter), Email: in.Email}
	p.Accounts[account.Subject] = &ProviderAccount{Account: account, Name: in.Name}
	return account, nil
}

// DeleteCreatedAccount implements usecase.AccountDirectory.
func (p *Provider) DeleteCreatedAccount(_ context.Context, subject string) error {
	if err := p.enter("DeleteCreatedAccount"); err != nil {
		return err
	}
	stored, ok := p.Accounts[subject]
	if !ok {
		return nil
	}
	if stored.Used {
		return usecase.ErrAccountInUse
	}
	delete(p.Accounts, subject)
	delete(p.Groups, subject)
	return nil
}

// AddToRoleGroup implements usecase.ProductAccess.
func (p *Provider) AddToRoleGroup(_ context.Context, subject string, role domain.Role) error {
	if err := p.enter("AddToRoleGroup"); err != nil {
		return err
	}
	p.Groups[subject] = role
	return nil
}

// RemoveFromProductGroups implements usecase.ProductAccess.
func (p *Provider) RemoveFromProductGroups(_ context.Context, subject string) error {
	if err := p.enter("RemoveFromProductGroups"); err != nil {
		return err
	}
	delete(p.Groups, subject)
	return nil
}

// MoveToRoleGroup implements usecase.ProductAccess.
func (p *Provider) MoveToRoleGroup(_ context.Context, subject string, _, to domain.Role) error {
	if err := p.enter("MoveToRoleGroup"); err != nil {
		return err
	}
	p.Groups[subject] = to
	return nil
}

// EndProductSessions implements usecase.ProductAccess.
func (p *Provider) EndProductSessions(_ context.Context, subject string) error {
	if err := p.enter("EndProductSessions"); err != nil {
		return err
	}
	p.EndedSessions = append(p.EndedSessions, subject)
	return nil
}

// SendActionEmail implements usecase.InviteSender.
func (p *Provider) SendActionEmail(_ context.Context, subject string, kind usecase.EmailKind) error {
	if err := p.enter("SendActionEmail"); err != nil {
		return err
	}
	p.Emails = append(p.Emails, SentEmail{Subject: subject, Kind: kind})
	return nil
}

// InGroup reports the product role group of the subject, if any.
func (p *Provider) InGroup(subject string) (domain.Role, bool) {
	role, ok := p.Groups[subject]
	return role, ok
}

// CallsOf counts how many times an operation was called.
func (p *Provider) CallsOf(operation string) int {
	return countOf(p.Calls, operation)
}

func countOf(calls []string, operation string) int {
	n := 0
	for _, call := range calls {
		if call == operation {
			n++
		}
	}
	return n
}

// FixedClock is a Clock that only moves when told to.
type FixedClock struct{ T time.Time }

// Now implements usecase.Clock.
func (c *FixedClock) Now() time.Time { return c.T }

// Advance moves the clock forward.
func (c *FixedClock) Advance(d time.Duration) { c.T = c.T.Add(d) }

// SequentialIDs generates predictable identifiers: id-1, id-2, ...
type SequentialIDs struct{ n int }

// NewID implements usecase.IDGenerator.
func (s *SequentialIDs) NewID() string {
	s.n++
	return fmt.Sprintf("id-%d", s.n)
}
