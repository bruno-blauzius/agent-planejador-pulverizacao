package httpadapter_test

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func invitationBody(email string, mutate func(map[string]any)) map[string]any {
	body := map[string]any{
		"name": "Carla Campos", "email": email, "role": "planning_operator",
		"company_id": companyAID, "farm_scope": map[string]any{"mode": "all"},
	}
	if mutate != nil {
		mutate(body)
	}
	return body
}

func TestCreateInvitation(t *testing.T) {
	t.Run("creates the invitation and reports the e-mail", func(t *testing.T) {
		e := newAPIEnv(t)
		got := e.call(adminAID, http.MethodPost, "/invitations", invitationBody("carla@a.test", nil))
		if got.status != http.StatusCreated {
			t.Fatalf("status = %d, body %s", got.status, got.raw)
		}
		if got.header.Get("X-Invitation-Email") != "sent" {
			t.Errorf("X-Invitation-Email = %q, want sent", got.header.Get("X-Invitation-Email"))
		}
		body := got.object(t)
		invitation, _ := body["invitation"].(map[string]any)
		if body["status"] != "invited" || body["email"] != "carla@a.test" || invitation["id"] == "" || invitation["expires_at"] == "" {
			t.Errorf("user = %v", body)
		}
		if len(e.idp.Emails) != 1 {
			t.Errorf("emails sent = %d, want 1", len(e.idp.Emails))
		}
	})

	t.Run("a mail failure keeps the invitation and says not_sent", func(t *testing.T) {
		e := newAPIEnv(t)
		e.idp.Fail("SendActionEmail", errors.New("smtp not configured"))
		got := e.call(adminAID, http.MethodPost, "/invitations", invitationBody("carla@a.test", nil))
		if got.status != http.StatusCreated || got.header.Get("X-Invitation-Email") != "not_sent" {
			t.Fatalf("status = %d, header %q, body %s", got.status, got.header.Get("X-Invitation-Email"), got.raw)
		}
		if !contains(e.logs.String(), "smtp not configured") {
			t.Errorf("the cause of the mail failure was not logged")
		}
	})

	t.Run("an existing verified realm account is reused without e-mail", func(t *testing.T) {
		e := newAPIEnv(t)
		e.idp.SeedAccount("outra@pessoa.test", true)
		got := e.call(adminAID, http.MethodPost, "/invitations", invitationBody("outra@pessoa.test", nil))
		if got.status != http.StatusCreated || got.header.Get("X-Invitation-Email") != "not_needed" {
			t.Errorf("status = %d, header %q, body %s", got.status, got.header.Get("X-Invitation-Email"), got.raw)
		}
	})

	tests := []struct {
		name      string
		as        string
		body      any
		setup     func(*apiEnv)
		wantCode  string
		want      int
		wantField string
	}{
		{name: "e-mail already registered", as: adminAID, body: invitationBody("olga@a.test", nil), want: 409, wantCode: "email_already_registered"},
		{name: "unverified realm account", as: adminAID, body: invitationBody("pendente@x.test", nil), setup: func(e *apiEnv) { e.idp.SeedAccount("pendente@x.test", false) }, want: 409, wantCode: "email_not_verified"},
		{name: "inactive company", as: superAdminID, body: invitationBody("n@a.test", nil), setup: func(e *apiEnv) {
			company := companyOf(e, companyAID)
			company.Status = domain.CompanyInactive
			e.db.Companies[company.ID] = company
		}, want: 409, wantCode: "company_inactive"},
		{name: "admin invites into another company", as: adminAID, body: invitationBody("n@b.test", func(b map[string]any) { b["company_id"] = companyBID }), want: 403, wantCode: "forbidden"},
		{name: "admin grants super admin", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { b["role"] = "super_admin" }), want: 403, wantCode: "forbidden"},
		{name: "operator may not invite", as: operatorAID, body: invitationBody("n@a.test", nil), want: 403, wantCode: "forbidden"},
		{name: "unknown company", as: superAdminID, body: invitationBody("n@a.test", func(b map[string]any) { b["company_id"] = missingID }), want: 422, wantCode: "validation_failed", wantField: "company_id"},
		{name: "invalid e-mail", as: adminAID, body: invitationBody("sem-arroba", nil), want: 422, wantCode: "validation_failed", wantField: "email"},
		{name: "blank name", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { b["name"] = "  " }), want: 422, wantCode: "validation_failed", wantField: "name"},
		{name: "unknown role", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { b["role"] = "wizard" }), want: 422, wantCode: "validation_failed", wantField: "role"},
		{name: "missing company", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { delete(b, "company_id") }), want: 422, wantCode: "validation_failed", wantField: "company_id"},
		{name: "missing farm scope", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { delete(b, "farm_scope") }), want: 422, wantCode: "validation_failed", wantField: "farm_scope"},
		{name: "unknown field", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) { b["password"] = "x" }), want: 422, wantCode: "validation_failed", wantField: "password"},
		{name: "farm of another company", as: adminAID, body: invitationBody("n@a.test", func(b map[string]any) {
			b["farm_scope"] = map[string]any{"mode": "selected", "farm_ids": []string{farmB1}}
		}), want: 422, wantCode: "validation_failed", wantField: "farm_scope.farm_ids"},
		{name: "provider down", as: adminAID, body: invitationBody("n@a.test", nil), setup: func(e *apiEnv) {
			e.idp.Fail("FindAccountByEmail", usecase.ErrIdentityProviderUnavailable)
		}, want: 502, wantCode: "identity_provider_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAPIEnv(t)
			if tt.setup != nil {
				tt.setup(e)
			}
			got := e.call(tt.as, http.MethodPost, "/invitations", tt.body)
			if got.status != tt.want || got.errorCode(t) != tt.wantCode {
				t.Fatalf("status = %d code %q, want %d %q, body %.200s", got.status, got.errorCode(t), tt.want, tt.wantCode, got.raw)
			}
			if tt.wantField != "" && !slices.Contains(got.errorFields(t), tt.wantField) {
				t.Errorf("details %v miss %q", got.errorFields(t), tt.wantField)
			}
		})
	}
}

func TestResendInvitation(t *testing.T) {
	const invitedID = "00000000-0000-4000-8000-000000000a10"

	t.Run("renews and reports the e-mail", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		got := e.call(adminAID, http.MethodPost, "/invitations/"+string(invitation.ID)+"/resend", nil)
		if got.status != http.StatusNoContent || got.header.Get("X-Invitation-Email") != "sent" {
			t.Fatalf("status = %d, header %q, body %s", got.status, got.header.Get("X-Invitation-Email"), got.raw)
		}
		if e.db.Invitations[invitation.ID].SendCount != 2 {
			t.Errorf("send count = %d, want 2", e.db.Invitations[invitation.ID].SendCount)
		}
	})

	t.Run("mail failure still answers 204 with not_sent", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		e.idp.Fail("SendActionEmail", errors.New("smtp down"))
		got := e.call(adminAID, http.MethodPost, "/invitations/"+string(invitation.ID)+"/resend", nil)
		if got.status != http.StatusNoContent || got.header.Get("X-Invitation-Email") != "not_sent" {
			t.Errorf("status = %d, header %q", got.status, got.header.Get("X-Invitation-Email"))
		}
	})

	t.Run("closed invitation is 409", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		if err := closeInvitation(e, invitation); err != nil {
			t.Fatal(err)
		}
		got := e.call(adminAID, http.MethodPost, "/invitations/"+string(invitation.ID)+"/resend", nil)
		if got.status != http.StatusConflict || got.errorCode(t) != "invitation_closed" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("other company, unknown, malformed and forbidden", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		for _, tt := range []struct {
			name, as, id string
			want         int
		}{
			{"admin of another company", adminBID, string(invitation.ID), 404},
			{"unknown id", adminAID, missingID, 404},
			{"malformed id", adminAID, "x", 404},
			{"operator", operatorAID, string(invitation.ID), 403},
		} {
			if got := e.call(tt.as, http.MethodPost, "/invitations/"+tt.id+"/resend", nil); got.status != tt.want {
				t.Errorf("%s: status = %d, want %d, body %s", tt.name, got.status, tt.want, got.raw)
			}
		}
	})

	t.Run("provider down is 502", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		e.idp.Fail("AddToRoleGroup", usecase.ErrIdentityProviderUnavailable)
		got := e.call(adminAID, http.MethodPost, "/invitations/"+string(invitation.ID)+"/resend", nil)
		if got.status != http.StatusBadGateway || got.errorCode(t) != "identity_provider_unavailable" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})
}

func TestCancelInvitation(t *testing.T) {
	const invitedID = "00000000-0000-4000-8000-000000000a10"

	t.Run("cancels and frees the e-mail", func(t *testing.T) {
		e := newAPIEnv(t)
		user, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		got := e.call(adminAID, http.MethodDelete, "/invitations/"+string(invitation.ID), nil)
		if got.status != http.StatusNoContent || len(got.raw) != 0 {
			t.Fatalf("status = %d, body %q", got.status, got.raw)
		}
		if after := e.call(adminAID, http.MethodGet, "/users/"+string(user.ID), nil); after.status != http.StatusNotFound {
			t.Errorf("cancelled user: status %d, want 404", after.status)
		}
	})

	t.Run("closed invitation is 409", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		if err := closeInvitation(e, invitation); err != nil {
			t.Fatal(err)
		}
		got := e.call(adminAID, http.MethodDelete, "/invitations/"+string(invitation.ID), nil)
		if got.status != http.StatusConflict || got.errorCode(t) != "invitation_closed" {
			t.Errorf("status = %d, body %s", got.status, got.raw)
		}
	})

	t.Run("other company, malformed and forbidden", func(t *testing.T) {
		e := newAPIEnv(t)
		_, invitation := e.addInvited(invitedID, companyOf(e, companyAID), "novo@a.test")
		for _, tt := range []struct {
			name, as, id string
			want         int
		}{
			{"admin of another company", adminBID, string(invitation.ID), 404},
			{"malformed id", adminAID, "x", 404},
			{"operator", operatorAID, string(invitation.ID), 403},
		} {
			if got := e.call(tt.as, http.MethodDelete, "/invitations/"+tt.id, nil); got.status != tt.want {
				t.Errorf("%s: status = %d, want %d", tt.name, got.status, tt.want)
			}
		}
	})
}

func closeInvitation(e *apiEnv, invitation domain.Invitation) error {
	accepted := e.db.Invitations[invitation.ID]
	if err := accepted.Accept(e.clock.T); err != nil {
		return err
	}
	e.db.Invitations[invitation.ID] = accepted
	return nil
}
