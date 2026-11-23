package keycloak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Required actions of an account created by the product: the person confirms
// the e-mail and chooses the password (no password is ever set here).
const (
	actionVerifyEmail    = "VERIFY_EMAIL"
	actionUpdatePassword = "UPDATE_PASSWORD"

	// fallbackLastName satisfies the mandatory lastName of the Keycloak 26 user
	// profile when the person has a single name (ADR 0015, D5).
	fallbackLastName = "-"
)

// subjectPattern guards the id used as a path segment: Keycloak ids are UUIDs.
var subjectPattern = regexp.MustCompile(`^[0-9A-Za-z-]{8,64}$`)

// Compile-time checks that the adapter satisfies the ports it serves.
var (
	_ usecase.AccountDirectory = (*Adapter)(nil)
	_ usecase.ProductAccess    = (*Adapter)(nil)
	_ usecase.InviteSender     = (*Adapter)(nil)
)

// Adapter talks to the Keycloak Admin REST API as the provisioner client.
// It is safe for concurrent use.
type Adapter struct {
	cfg    Config
	client *client
	issuer string

	groupMu  sync.Mutex
	groupIDs map[domain.Role]string // cache of sub group ids by role
}

// New validates the configuration and returns the adapter. No network call is made.
func New(cfg Config) (*Adapter, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, fmt.Errorf("keycloak adapter config: %w", err)
	}
	return &Adapter{
		cfg:      cfg,
		client:   newClient(cfg),
		issuer:   cfg.IssuerURL + "/realms/" + cfg.Realm,
		groupIDs: map[domain.Role]string{},
	}, nil
}

// Issuer is the "iss" of the realm tokens, as reported in Account.Issuer.
func (a *Adapter) Issuer() string { return a.issuer }

// ---- representations -----------------------------------------------------------------

type userRepresentation struct {
	ID               string   `json:"id,omitempty"`
	Username         string   `json:"username,omitempty"`
	Email            string   `json:"email,omitempty"`
	FirstName        string   `json:"firstName,omitempty"`
	LastName         string   `json:"lastName,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
	EmailVerified    *bool    `json:"emailVerified,omitempty"`
	RequiredActions  []string `json:"requiredActions,omitempty"`
	CreatedTimestamp int64    `json:"createdTimestamp,omitempty"`
}

type groupRepresentation struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type sessionRepresentation struct {
	ID string `json:"id"`
	// Clients maps the client UUID to its clientId for every client of the SSO session.
	Clients map[string]string `json:"clients"`
}

// AccountDetails is what the provider knows about an account beyond the port's Account.
type AccountDetails struct {
	usecase.Account
	Enabled         bool
	CreatedAt       time.Time
	RequiredActions []string
}

func (u userRepresentation) enabled() bool { return u.Enabled == nil || *u.Enabled }

func (u userRepresentation) verified() bool { return u.EmailVerified != nil && *u.EmailVerified }

// ---- AccountDirectory ----------------------------------------------------------------

// FindAccountByEmail implements usecase.AccountDirectory (exact e-mail match).
func (a *Adapter) FindAccountByEmail(ctx context.Context, email domain.Email) (usecase.Account, error) {
	details, err := a.FindAccountDetailsByEmail(ctx, email)
	if err != nil {
		return usecase.Account{}, err
	}
	return details.Account, nil
}

// FindAccountDetailsByEmail is FindAccountByEmail with the extra fields
// (enabled flag, creation time, pending required actions).
func (a *Adapter) FindAccountDetailsByEmail(ctx context.Context, email domain.Email) (AccountDetails, error) {
	const operation = "find user by e-mail"
	var users []userRepresentation
	_, err := a.client.do(ctx, request{
		operation: operation,
		method:    http.MethodGet,
		url:       a.client.adminURL("users"),
		query:     url.Values{"email": {email.String()}, "exact": {"true"}, "briefRepresentation": {"false"}},
		out:       &users,
	})
	if err != nil {
		return AccountDetails{}, unavailable(operation, err)
	}
	for _, user := range users {
		// "exact" is honored by Keycloak 26; the comparison keeps the contract if it is not.
		if strings.EqualFold(user.Email, email.String()) && user.ID != "" {
			return a.details(user), nil
		}
	}
	return AccountDetails{}, usecase.ErrAccountNotFound
}

func (a *Adapter) details(user userRepresentation) AccountDetails {
	details := AccountDetails{
		Account: usecase.Account{
			Issuer:        a.issuer,
			Subject:       user.ID,
			Email:         domain.Email(strings.ToLower(user.Email)),
			EmailVerified: user.verified(),
		},
		Enabled:         user.enabled(),
		RequiredActions: user.RequiredActions,
	}
	if user.CreatedTimestamp > 0 {
		details.CreatedAt = time.UnixMilli(user.CreatedTimestamp).UTC()
	}
	return details
}

// CreateAccount implements usecase.AccountDirectory. The account has
// username = e-mail, emailVerified=false, the required actions VERIFY_EMAIL and
// UPDATE_PASSWORD and no credentials. It does not send e-mail.
func (a *Adapter) CreateAccount(ctx context.Context, in usecase.NewAccount) (usecase.Account, error) {
	const operation = "create user"
	first, last := splitName(in.Name)
	enabled, verified := true, false
	body := userRepresentation{
		Username:        in.Email.String(),
		Email:           in.Email.String(),
		FirstName:       first,
		LastName:        last,
		Enabled:         &enabled,
		EmailVerified:   &verified,
		RequiredActions: []string{actionVerifyEmail, actionUpdatePassword},
	}
	resp, err := a.client.do(ctx, request{
		operation: operation,
		method:    http.MethodPost,
		url:       a.client.adminURL("users"),
		body:      body,
	})
	if err != nil {
		if statusOf(err) == http.StatusConflict {
			return usecase.Account{}, unavailable(operation, errors.Join(ErrAccountExists, err))
		}
		return usecase.Account{}, unavailable(operation, err)
	}
	id := idFromLocation(resp.header.Get("Location"))
	if id == "" {
		// Defensive: Keycloak always answers 201 with Location. Resolve by e-mail.
		found, findErr := a.FindAccountByEmail(ctx, in.Email)
		if findErr != nil {
			return usecase.Account{}, unavailable(operation, errors.New("created user has no id"))
		}
		return found, nil
	}
	return usecase.Account{Issuer: a.issuer, Subject: id, Email: in.Email, EmailVerified: false}, nil
}

// splitName returns firstName (first word) and lastName (the rest), with a
// fallback last name when there is a single word.
func splitName(name string) (first, last string) {
	words := strings.Fields(name)
	switch len(words) {
	case 0:
		return fallbackLastName, fallbackLastName
	case 1:
		return words[0], fallbackLastName
	default:
		return words[0], strings.Join(words[1:], " ")
	}
}

func idFromLocation(location string) string {
	if location == "" {
		return ""
	}
	id := location[strings.LastIndex(location, "/")+1:]
	if !subjectPattern.MatchString(id) {
		return ""
	}
	return id
}

// DeleteCreatedAccount implements usecase.AccountDirectory.
//
// The port only receives the subject, so "created by this flow" is decided by
// evidence that nobody ever used the account. It returns ErrAccountInUse when ANY of:
//   - the account has a credential (a person or an admin set a password; the
//     product creates accounts without one);
//   - the e-mail is verified (the link was used, or another product verified it);
//   - there is an active SSO session or a linked federated identity;
//   - the user belongs to a group outside the product tree (another product
//     uses it);
//   - a LOGIN event exists (only when the provisioner may read events: it
//     does not hold view-events in the default setup, so a 403 is ignored).
//
// A missing account is success (idempotent). The window between the checks
// and the DELETE is accepted: the callers run seconds after creation.
func (a *Adapter) DeleteCreatedAccount(ctx context.Context, subject string) error {
	const operation = "delete created user"
	if err := checkSubject(subject); err != nil {
		return err
	}
	var user userRepresentation
	_, err := a.client.do(ctx, request{
		operation: "get user", method: http.MethodGet,
		url: a.client.adminURL("users", subject), out: &user,
	})
	if statusOf(err) == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return unavailable(operation, err)
	}
	if user.verified() {
		return usecase.ErrAccountInUse
	}

	inUse, err := a.hasBeenUsed(ctx, subject)
	if err != nil {
		return unavailable(operation, err)
	}
	if inUse {
		return usecase.ErrAccountInUse
	}

	_, err = a.client.do(ctx, request{
		operation: operation, method: http.MethodDelete,
		url: a.client.adminURL("users", subject),
	})
	if err != nil && statusOf(err) != http.StatusNotFound {
		return unavailable(operation, err)
	}
	return nil
}

// hasBeenUsed applies the heuristic documented on DeleteCreatedAccount.
func (a *Adapter) hasBeenUsed(ctx context.Context, subject string) (bool, error) {
	var credentials []struct {
		ID string `json:"id"`
	}
	if _, err := a.client.do(ctx, request{
		operation: "list credentials", method: http.MethodGet,
		url: a.client.adminURL("users", subject, "credentials"), out: &credentials,
	}); err != nil {
		return false, err
	}
	if len(credentials) > 0 {
		return true, nil
	}

	var federated []struct {
		IdentityProvider string `json:"identityProvider"`
	}
	if _, err := a.client.do(ctx, request{
		operation: "list federated identities", method: http.MethodGet,
		url: a.client.adminURL("users", subject, "federated-identity"), out: &federated,
	}); err != nil {
		return false, err
	}
	if len(federated) > 0 {
		return true, nil
	}

	var sessions []sessionRepresentation
	if _, err := a.client.do(ctx, request{
		operation: "list sessions", method: http.MethodGet,
		url: a.client.adminURL("users", subject, "sessions"), out: &sessions,
	}); err != nil {
		return false, err
	}
	if len(sessions) > 0 {
		return true, nil
	}

	groups, err := a.userGroups(ctx, subject)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if !a.isProductPath(group.Path) {
			return true, nil
		}
	}

	var events []struct {
		Type string `json:"type"`
	}
	_, err = a.client.do(ctx, request{
		operation: "list login events", method: http.MethodGet,
		url:   a.client.adminURL("events"),
		query: url.Values{"user": {subject}, "type": {"LOGIN"}, "max": {"1"}},
		out:   &events,
	})
	switch statusOf(err) {
	case 0:
		if err != nil {
			return false, err
		}
		return len(events) > 0, nil
	case http.StatusForbidden, http.StatusBadRequest, http.StatusNotFound:
		return false, nil // events not readable by the provisioner: no extra evidence
	default:
		return false, err
	}
}

// ---- ProductAccess -------------------------------------------------------------------

// AddToRoleGroup implements usecase.ProductAccess. It guarantees exactly one
// role sub group: the target is joined and any other product role group is left.
func (a *Adapter) AddToRoleGroup(ctx context.Context, subject string, role domain.Role) error {
	return a.setRoleGroup(ctx, "add to role group", subject, role)
}

// MoveToRoleGroup implements usecase.ProductAccess. The "from" role is only
// informative: the result is the same as AddToRoleGroup(to), which also
// repairs a user found in a different group than the caller expected.
func (a *Adapter) MoveToRoleGroup(ctx context.Context, subject string, from, to domain.Role) error {
	if !from.Valid() {
		return fmt.Errorf("move to role group: invalid origin role %q", from)
	}
	return a.setRoleGroup(ctx, "move to role group", subject, to)
}

// RemoveFromProductGroups implements usecase.ProductAccess. Only the product
// role groups are touched; memberships in other groups stay. A missing user is success.
func (a *Adapter) RemoveFromProductGroups(ctx context.Context, subject string) error {
	const operation = "remove from product groups"
	if err := checkSubject(subject); err != nil {
		return err
	}
	groups, err := a.userGroups(ctx, subject)
	if statusOf(err) == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return unavailable(operation, err)
	}
	for _, group := range groups {
		if !a.isRoleGroupPath(group.Path) {
			continue
		}
		if err := a.leaveGroup(ctx, operation, subject, group.ID); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) setRoleGroup(ctx context.Context, operation, subject string, role domain.Role) error {
	if err := checkSubject(subject); err != nil {
		return err
	}
	if !role.Valid() {
		return fmt.Errorf("%s: invalid role %q", operation, role)
	}
	targetID, err := a.roleGroupID(ctx, role)
	if err != nil {
		return err
	}
	groups, err := a.userGroups(ctx, subject)
	if statusOf(err) == http.StatusNotFound {
		return fmt.Errorf("%s: %w", operation, usecase.ErrAccountNotFound)
	}
	if err != nil {
		return unavailable(operation, err)
	}

	member := false
	var others []string
	for _, group := range groups {
		switch {
		case group.ID == targetID:
			member = true
		case a.isRoleGroupPath(group.Path):
			others = append(others, group.ID)
		}
	}
	// Join the target first, leave the others after: a failure in between leaves
	// the account in two role groups (the token is then ambiguous and refused,
	// and a retry finishes the job) instead of in none, which would silently
	// lock the person out.
	if !member {
		if err := a.joinGroup(ctx, operation, subject, role, targetID); err != nil {
			return err
		}
	}
	for _, groupID := range others {
		if err := a.leaveGroup(ctx, operation, subject, groupID); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) joinGroup(ctx context.Context, operation, subject string, role domain.Role, groupID string) error {
	_, err := a.client.do(ctx, request{
		operation: operation, method: http.MethodPut,
		url: a.client.adminURL("users", subject, "groups", groupID),
	})
	if statusOf(err) == http.StatusNotFound {
		a.forgetGroup(role) // the group may have been recreated: resolve it again next time
	}
	return unavailable(operation, err)
}

func (a *Adapter) leaveGroup(ctx context.Context, operation, subject, groupID string) error {
	_, err := a.client.do(ctx, request{
		operation: operation, method: http.MethodDelete,
		url: a.client.adminURL("users", subject, "groups", groupID),
	})
	if err != nil && statusOf(err) != http.StatusNotFound {
		return unavailable(operation, err)
	}
	return nil
}

func (a *Adapter) userGroups(ctx context.Context, subject string) ([]groupRepresentation, error) {
	var groups []groupRepresentation
	_, err := a.client.do(ctx, request{
		operation: "list user groups", method: http.MethodGet,
		url:   a.client.adminURL("users", subject, "groups"),
		query: url.Values{"briefRepresentation": {"true"}},
		out:   &groups,
	})
	return groups, err
}

func (a *Adapter) rolePath(role domain.Role) string {
	return a.cfg.GroupBasePath + "/" + a.cfg.RoleGroups[role]
}

func (a *Adapter) isRoleGroupPath(path string) bool {
	for _, role := range domain.Roles() {
		if path == a.rolePath(role) {
			return true
		}
	}
	return false
}

// isProductPath is true for the product root group and anything below it.
func (a *Adapter) isProductPath(path string) bool {
	return path == a.cfg.GroupBasePath || strings.HasPrefix(path, a.cfg.GroupBasePath+"/")
}

// roleGroupID resolves (and caches) the id of the sub group of a role by path.
func (a *Adapter) roleGroupID(ctx context.Context, role domain.Role) (string, error) {
	a.groupMu.Lock()
	cached, ok := a.groupIDs[role]
	a.groupMu.Unlock()
	if ok {
		return cached, nil
	}

	const operation = "resolve role group"
	segments := strings.Split(strings.TrimPrefix(a.rolePath(role), "/"), "/")
	endpoint := a.client.adminURL(append([]string{"group-by-path"}, segments...)...)
	var group groupRepresentation
	if _, err := a.client.do(ctx, request{
		operation: operation, method: http.MethodGet, url: endpoint, out: &group,
	}); err != nil {
		return "", unavailable(operation+" "+a.rolePath(role), err)
	}
	if group.ID == "" || group.Path != a.rolePath(role) {
		return "", unavailable(operation, errors.New("group path not found in the realm"))
	}
	a.groupMu.Lock()
	a.groupIDs[role] = group.ID
	a.groupMu.Unlock()
	return group.ID, nil
}

func (a *Adapter) forgetGroup(role domain.Role) {
	a.groupMu.Lock()
	delete(a.groupIDs, role)
	a.groupMu.Unlock()
}

// EndProductSessions implements usecase.ProductAccess.
//
// Keycloak sessions are SSO sessions that may include several clients, and the
// Admin API can only delete a whole session. To honor "never touch other
// products' sessions", only sessions whose sole client is the product client
// are deleted; a session shared with another client is left alone. That gap
// is closed by the other steps of deactivation: the API revokes its own
// auth_sessions and the user leaves the product groups, so the next token
// refresh carries no role of the product.
func (a *Adapter) EndProductSessions(ctx context.Context, subject string) error {
	const operation = "end product sessions"
	if err := checkSubject(subject); err != nil {
		return err
	}
	var sessions []sessionRepresentation
	_, err := a.client.do(ctx, request{
		operation: "list sessions", method: http.MethodGet,
		url: a.client.adminURL("users", subject, "sessions"), out: &sessions,
	})
	if statusOf(err) == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return unavailable(operation, err)
	}
	for _, session := range sessions {
		if session.ID == "" || !onlyClient(session.Clients, a.cfg.ProductClientID) {
			continue
		}
		_, err := a.client.do(ctx, request{
			operation: operation, method: http.MethodDelete,
			url:   a.client.adminURL("sessions", session.ID),
			query: url.Values{"isOffline": {"false"}},
		})
		if err != nil && statusOf(err) != http.StatusNotFound {
			return unavailable(operation, err)
		}
	}
	return nil
}

func onlyClient(clients map[string]string, clientID string) bool {
	if len(clients) == 0 {
		return false
	}
	for _, id := range clients {
		if id != clientID {
			return false
		}
	}
	return true
}

// ---- InviteSender --------------------------------------------------------------------

// SendActionEmail implements usecase.InviteSender with execute-actions-email
// (client_id of the product, redirect_uri and lifespan from the configuration).
//
// A failure that means "the e-mail did not leave" (SMTP missing or failing:
// Keycloak answers 500, or 400 for a rejected redirect) is returned wrapping
// ErrEmailNotSent so use cases report not_sent and keep the invitation valid.
// Authorization failures and network errors are ErrIdentityProviderUnavailable.
func (a *Adapter) SendActionEmail(ctx context.Context, subject string, kind usecase.EmailKind) error {
	const operation = "send action e-mail"
	if err := checkSubject(subject); err != nil {
		return err
	}
	actions, err := actionsFor(kind)
	if err != nil {
		return err
	}
	query := url.Values{
		"client_id": {a.cfg.ProductClientID},
		"lifespan":  {strconv.FormatInt(int64(a.cfg.ActionLifespan/time.Second), 10)},
	}
	if a.cfg.RedirectURI != "" {
		query.Set("redirect_uri", a.cfg.RedirectURI)
	}
	_, err = a.client.do(ctx, request{
		operation: operation, method: http.MethodPut,
		url:   a.client.adminURL("users", subject, "execute-actions-email"),
		query: query, body: actions,
	})
	switch status := statusOf(err); {
	case err == nil:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%s: %w", operation, usecase.ErrAccountNotFound)
	case status == http.StatusBadRequest || status == http.StatusInternalServerError:
		return fmt.Errorf("%w: %s: %w", ErrEmailNotSent, operation, err)
	default:
		return unavailable(operation, err)
	}
}

func actionsFor(kind usecase.EmailKind) ([]string, error) {
	switch kind {
	case usecase.EmailSetupAccount:
		return []string{actionVerifyEmail, actionUpdatePassword}, nil
	case usecase.EmailVerifyEmail:
		return []string{actionVerifyEmail}, nil
	default:
		return nil, fmt.Errorf("send action e-mail: unknown e-mail kind %q", kind)
	}
}

func checkSubject(subject string) error {
	if !subjectPattern.MatchString(subject) {
		return errors.New("invalid account subject")
	}
	return nil
}
