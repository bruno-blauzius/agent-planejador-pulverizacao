//go:build integration

package postgres_test

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func audit(company domain.Company, entity domain.UserID, action string) domain.AuditEvent {
	return domain.AuditEvent{
		CompanyID: company.ID, ActorKind: domain.ActorSystem, ActorName: "system",
		Action: action, EntityType: domain.AuditEntityUser, EntityID: string(entity),
		Metadata: map[string]string{"reason": "teste"},
	}
}

func auditCount(t *testing.T, entity domain.UserID) int {
	t.Helper()
	var n int
	must(t, testPool.QueryRow(ctx(), `SELECT count(*) FROM audit_events WHERE entity_id = $1`, string(entity)).Scan(&n))
	return n
}

func TestAuditIsAppendOnly(t *testing.T) {
	company := newCompany(t)
	user := newUser(t, company)
	actor := user.ID
	event := audit(company, user.ID, domain.AuditUserRenamed)
	event.ActorKind, event.ActorUserID, event.ActorName = domain.ActorUser, &actor, user.Name
	must(t, testDB.Record(ctx(), event))
	must(t, testDB.Record(ctx(), audit(company, user.ID, domain.AuditUserDeactivated))) // system actor, nil metadata ok
	if n := auditCount(t, user.ID); n != 2 {
		t.Fatalf("audit rows = %d", n)
	}
	var metadata string
	must(t, testPool.QueryRow(ctx(), `SELECT metadata::text FROM audit_events WHERE entity_id = $1 ORDER BY id LIMIT 1`, string(user.ID)).Scan(&metadata))
	if metadata != `{"reason": "teste"}` {
		t.Fatalf("metadata = %s", metadata)
	}

	// the runtime role cannot rewrite history
	for name, statement := range map[string]string{
		"update":   `UPDATE audit_events SET action = 'forged' WHERE entity_id = $1`,
		"delete":   `DELETE FROM audit_events WHERE entity_id = $1`,
		"truncate": `TRUNCATE audit_events`,
	} {
		var err error
		if name == "truncate" {
			_, err = testPool.Exec(ctx(), statement)
		} else {
			_, err = testPool.Exec(ctx(), statement, string(user.ID))
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s must be denied with insufficient_privilege, got %v", name, err)
		}
	}
	if n := auditCount(t, user.ID); n != 2 {
		t.Fatalf("history changed: %d rows", n)
	}

	bad := audit(company, user.ID, "")
	if err := testDB.Record(ctx(), bad); err == nil {
		t.Fatal("empty action must violate the check constraint")
	}
}

func sessionFixture(t *testing.T, user domain.User, issuer string) (usecase.SessionRecord, [32]byte) {
	t.Helper()
	hash := sha256.Sum256([]byte(newID()))
	return usecase.SessionRecord{
		ID: newID(), UserID: user.ID, Issuer: issuer, IdPSessionID: newID(),
		GrantedRoles:    []string{"admin"},
		AccessExpiresAt: time.Now().Add(5 * time.Minute).UTC().Truncate(time.Microsecond),
		ExpiresAt:       time.Now().Add(8 * time.Hour).UTC().Truncate(time.Microsecond),
		RefreshTokenEnc: []byte("cipher-1"), EncKeyID: 1,
	}, hash
}

func TestSessionLifecycle(t *testing.T) {
	company := newCompany(t)
	user := newUser(t, company)
	issuer := "https://idp/realms/inceres"
	must(t, testDB.LinkIdentity(ctx(), domain.Identity{UserID: user.ID, Issuer: issuer, Subject: newID()}))

	record, hash := sessionFixture(t, user, issuer)
	must(t, testDB.CreateSession(ctx(), record, hash))
	if err := testDB.CreateSession(ctx(), record, hash); err == nil {
		t.Fatal("the same session id or token hash must not be stored twice")
	}
	var loginAt *time.Time
	must(t, testPool.QueryRow(ctx(), `SELECT last_login_at FROM user_identities WHERE user_id = $1`, string(user.ID)).Scan(&loginAt))
	if loginAt == nil {
		t.Fatal("creating a session must stamp last_login_at on the identity")
	}

	found, err := testDB.FindSessionByTokenHash(ctx(), hash)
	must(t, err)
	if found.ID != record.ID || found.UserID != user.ID || found.IdPSessionID != record.IdPSessionID ||
		string(found.RefreshTokenEnc) != "cipher-1" || found.EncKeyID != 1 || len(found.GrantedRoles) != 1 ||
		!found.AccessExpiresAt.Equal(record.AccessExpiresAt) || found.CreatedAt.IsZero() {
		t.Fatalf("found = %+v", found)
	}
	_, err = testDB.FindSessionByTokenHash(ctx(), sha256.Sum256([]byte("unknown")))
	requireIs(t, err, usecase.ErrNotFound)

	// renew with a new refresh token, then without (keeps the stored one and its key)
	newAccess := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond)
	must(t, testDB.RenewSession(ctx(), record.ID, []string{"planning_operator"}, newAccess, []byte("cipher-2"), 2))
	must(t, testDB.RenewSession(ctx(), record.ID, []string{"admin"}, newAccess, nil, 9))
	found, err = testDB.FindSessionByTokenHash(ctx(), hash)
	must(t, err)
	if string(found.RefreshTokenEnc) != "cipher-2" || found.EncKeyID != 2 || found.GrantedRoles[0] != "admin" || !found.AccessExpiresAt.Equal(newAccess) {
		t.Fatalf("after renew = %+v", found)
	}
	requireIs(t, testDB.RenewSession(ctx(), newID(), nil, newAccess, nil, 1), usecase.ErrNotFound)

	// touch is rate limited by the configured interval (one hour in these tests)
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	must(t, testDB.TouchSession(ctx(), record.ID, t0))
	must(t, testDB.TouchSession(ctx(), record.ID, t0.Add(10*time.Minute)))
	found, _ = testDB.FindSessionByTokenHash(ctx(), hash)
	if found.LastSeenAt == nil || !found.LastSeenAt.Equal(t0) {
		t.Fatalf("a touch inside the interval must not write: %v", found.LastSeenAt)
	}
	must(t, testDB.TouchSession(ctx(), record.ID, t0.Add(2*time.Hour)))
	found, _ = testDB.FindSessionByTokenHash(ctx(), hash)
	if !found.LastSeenAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("a touch after the interval must write: %v", found.LastSeenAt)
	}

	must(t, testDB.RevokeSession(ctx(), record.ID, time.Now()))
	must(t, testDB.RevokeSession(ctx(), record.ID, time.Now())) // idempotent
	_, err = testDB.FindSessionByTokenHash(ctx(), hash)
	requireIs(t, err, usecase.ErrNotFound)
	requireIs(t, testDB.RenewSession(ctx(), record.ID, nil, newAccess, nil, 1), usecase.ErrNotFound)
}

func TestSessionExpiryAndBulkRevocation(t *testing.T) {
	company, other := newCompany(t), newCompany(t)
	user, colleague, stranger := newUser(t, company), newUser(t, company), newUser(t, other)
	issuer := "https://idp/realms/inceres"

	expired, expiredHash := sessionFixture(t, user, issuer)
	expired.ExpiresAt = time.Now().Add(-time.Minute).UTC()
	must(t, testDB.CreateSession(ctx(), expired, expiredHash))
	_, err := testDB.FindSessionByTokenHash(ctx(), expiredHash)
	requireIs(t, err, usecase.ErrNotFound)

	create := func(u domain.User) ([32]byte, usecase.SessionRecord) {
		rec, h := sessionFixture(t, u, issuer)
		must(t, testDB.CreateSession(ctx(), rec, h))
		return h, rec
	}
	alive := func(h [32]byte) bool {
		_, err := testDB.FindSessionByTokenHash(ctx(), h)
		return err == nil
	}

	// by IdP session id (back-channel logout), scoped to the issuer
	hSid, recSid := create(user)
	hOtherIssuer, _ := create(user)
	must(t, testDB.RevokeSessionsByIdPSession(ctx(), "https://another-issuer", recSid.IdPSessionID, time.Now()))
	if !alive(hSid) {
		t.Fatal("another issuer's sid must not revoke this session")
	}
	must(t, testDB.RevokeSessionsByIdPSession(ctx(), issuer, recSid.IdPSessionID, time.Now()))
	if alive(hSid) || !alive(hOtherIssuer) {
		t.Fatal("only the session with that sid must end")
	}

	// by user and by company
	hUser, _ := create(user)
	hColleague, _ := create(colleague)
	hStranger, _ := create(stranger)
	must(t, testDB.RevokeUserSessions(ctx(), user.ID, time.Now()))
	if alive(hUser) || alive(hOtherIssuer) || !alive(hColleague) {
		t.Fatal("RevokeUserSessions must end every session of that user and nobody else's")
	}
	must(t, testDB.RevokeCompanySessions(ctx(), company.ID, time.Now()))
	if alive(hColleague) {
		t.Fatal("RevokeCompanySessions must end the company's sessions")
	}
	if !alive(hStranger) {
		t.Fatal("RevokeCompanySessions must not touch another company")
	}

	// forcing a renewal: the access expiry moves to the given time, never later
	hRenew, recRenew := create(stranger)
	now := time.Now().UTC().Truncate(time.Microsecond)
	must(t, testDB.ExpireUserAccess(ctx(), stranger.ID, now))
	found, err := testDB.FindSessionByTokenHash(ctx(), hRenew)
	must(t, err)
	if !found.AccessExpiresAt.Equal(now) || found.ID != recRenew.ID {
		t.Fatalf("access expiry = %v, want %v", found.AccessExpiresAt, now)
	}
}
