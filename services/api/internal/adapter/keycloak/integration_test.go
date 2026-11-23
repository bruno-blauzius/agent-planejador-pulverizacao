//go:build integration || keycloak

// Integration test against the local Keycloak (infra-docker, realm inceres).
// Run with `make api-test-keycloak`. It reads the provisioner secret from the
// repository .env (or the environment) and never prints it.
package keycloak

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// loadDotEnv reads KEY=VALUE lines of the repository .env without overriding
// variables already set in the environment.
func loadDotEnv(t *testing.T) map[string]string {
	t.Helper()
	values := map[string]string{}
	file, err := os.Open(filepath.Join("..", "..", "..", "..", "..", ".env"))
	if err != nil {
		return values
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Logf("close .env: %v", err)
		}
	}()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return values
}

func TestKeycloakLifecycle(t *testing.T) {
	dotenv := loadDotEnv(t)
	lookup := func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
		value, ok := dotenv[key]
		return value, ok
	}
	if secret, _ := lookup("KEYCLOAK_PROVISIONER_CLIENT_SECRET"); secret == "" {
		t.Skip("KEYCLOAK_PROVISIONER_CLIENT_SECRET not set (run `make keycloak` first)")
	}
	cfg, err := ConfigFromEnv(lookup)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	email, err := domain.ParseEmail("kc-it-" + hex.EncodeToString(suffix) + "@agentes-agro.test")
	if err != nil {
		t.Fatal(err)
	}

	var subject string
	t.Cleanup(func() {
		// Cleanup runs even after a failure and must not depend on the adapter
		// heuristics: remove the group memberships, then the user, directly.
		if subject == "" {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = adapter.RemoveFromProductGroups(cleanCtx, subject)
		_, err := adapter.client.do(cleanCtx, request{
			operation: "cleanup delete user", method: http.MethodDelete,
			url: adapter.client.adminURL("users", subject),
		})
		if err != nil && statusOf(err) != http.StatusNotFound {
			t.Errorf("cleanup failed, delete %s by hand: %v", email, err)
		}
	})

	if _, err := adapter.FindAccountByEmail(ctx, email); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("fresh e-mail must not exist: %v", err)
	}

	account, err := adapter.CreateAccount(ctx, usecase.NewAccount{Email: email, Name: "Teste Integracao Keycloak"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	subject = account.Subject

	found, err := adapter.FindAccountDetailsByEmail(ctx, email)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found.Subject != subject || found.EmailVerified || !found.Enabled || found.Issuer != adapter.Issuer() {
		t.Fatalf("unexpected account: %+v", found)
	}
	if got := strings.Join(found.RequiredActions, ","); !strings.Contains(got, "VERIFY_EMAIL") || !strings.Contains(got, "UPDATE_PASSWORD") {
		t.Fatalf("required actions = %v", found.RequiredActions)
	}

	if err := adapter.AddToRoleGroup(ctx, subject, domain.RolePlanningOperator); err != nil {
		t.Fatalf("add to group: %v", err)
	}
	if err := adapter.AddToRoleGroup(ctx, subject, domain.RolePlanningOperator); err != nil {
		t.Fatalf("add to group again: %v", err)
	}
	assertRoleGroups(ctx, t, adapter, subject, "/agentes-agro/planning-operators")

	if err := adapter.MoveToRoleGroup(ctx, subject, domain.RolePlanningOperator, domain.RoleAdmin); err != nil {
		t.Fatalf("move: %v", err)
	}
	assertRoleGroups(ctx, t, adapter, subject, "/agentes-agro/admins")

	if err := adapter.EndProductSessions(ctx, subject); err != nil {
		t.Fatalf("end sessions: %v", err)
	}

	// SMTP is not configured by default in the dev realm: both outcomes are valid,
	// but a failure must be the honest "not sent", never "unavailable".
	if err := adapter.SendActionEmail(ctx, subject, usecase.EmailSetupAccount); err != nil && !errors.Is(err, ErrEmailNotSent) {
		t.Fatalf("send action e-mail: %v", err)
	}

	if err := adapter.RemoveFromProductGroups(ctx, subject); err != nil {
		t.Fatalf("remove from groups: %v", err)
	}
	assertRoleGroups(ctx, t, adapter, subject)

	if err := adapter.DeleteCreatedAccount(ctx, subject); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := adapter.FindAccountByEmail(ctx, email); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("deleted account must be gone: %v", err)
	}
	if err := adapter.DeleteCreatedAccount(ctx, subject); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
}

func assertRoleGroups(ctx context.Context, t *testing.T, adapter *Adapter, subject string, want ...string) {
	t.Helper()
	groups, err := adapter.userGroups(ctx, subject)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	var got []string
	for _, group := range groups {
		if adapter.isRoleGroupPath(group.Path) {
			got = append(got, group.Path)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("role groups = %v, want %v", got, want)
	}
}
