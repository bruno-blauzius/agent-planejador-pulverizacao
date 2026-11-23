package keycloak

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	testRealm  = "inceres"
	testSecret = "s3cr3t-provisioner-value-0123456789abcdef"
	testToken  = "tok-ABCDEF-secret-token"
)

// fakeUser is a user of the fake realm.
type fakeUser struct {
	ID              string
	Email           string
	FirstName       string
	LastName        string
	EmailVerified   bool
	Enabled         bool
	RequiredActions []string
	Groups          map[string]bool // group id
	Credentials     int
	Federated       int
	Sessions        []fakeSession
	LoginEvents     int
}

type fakeSession struct {
	ID      string
	Clients map[string]string // uuid -> clientId
}

// fakeKeycloak simulates the parts of the Admin API the adapter uses.
type fakeKeycloak struct {
	t      *testing.T
	server *httptest.Server

	mu     sync.Mutex
	users  map[string]*fakeUser
	groups map[string]string // path -> id
	next   int

	tokenCalls  int
	tokenTTL    int
	rejectToken bool // answers 401 once to the next admin call
	calls       []string
	lastEmail   struct {
		query   map[string]string
		actions []string
	}
	smtpBroken   bool
	eventsDenied bool
	// failures: "METHOD /path-suffix" -> status, answered before anything else.
	failures map[string]int
	// failCount limits how many times a failure applies (0 = forever).
	failCount map[string]int
}

func newFakeKeycloak(t *testing.T) *fakeKeycloak {
	t.Helper()
	f := &fakeKeycloak{
		t:         t,
		users:     map[string]*fakeUser{},
		tokenTTL:  300,
		failures:  map[string]int{},
		failCount: map[string]int{},
		groups: map[string]string{
			"/agentes-agro":                    "g-root",
			"/agentes-agro/super-admins":       "g-super",
			"/agentes-agro/admins":             "g-admin",
			"/agentes-agro/planning-operators": "g-operator",
			"/outro-produto/equipe":            "g-other",
		},
		eventsDenied: true,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeKeycloak) addUser(email string) *fakeUser {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addUserLocked(email)
}

func (f *fakeKeycloak) addUserLocked(email string) *fakeUser {
	f.next++
	user := &fakeUser{
		ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", f.next), Email: email,
		Enabled: true, Groups: map[string]bool{},
	}
	f.users[user.ID] = user
	return user
}

func (f *fakeKeycloak) user(id string) *fakeUser {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.users[id]
}

func (f *fakeKeycloak) failWith(key string, status, times int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[key] = status
	f.failCount[key] = times
}

func (f *fakeKeycloak) callsOf(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeKeycloak) groupPath(id string) string {
	for path, groupID := range f.groups {
		if groupID == id {
			return path
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (f *fakeKeycloak) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	tokenPath := "/realms/" + testRealm + "/protocol/openid-connect/token"
	if r.URL.Path == tokenPath {
		f.tokenCalls++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != DefaultProvisionerClientID ||
			r.Form.Get("client_secret") != testSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized_client", "detail": testSecret})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"access_token": testToken, "expires_in": f.tokenTTL, "token_type": "Bearer"})
		return
	}

	prefix := "/admin/realms/" + testRealm
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "HTTP 401 Unauthorized"})
		return
	}
	if f.rejectToken {
		f.rejectToken = false
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token expired"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, prefix)
	f.calls = append(f.calls, r.Method+" "+path)

	for key, status := range f.failures {
		method, suffix, _ := strings.Cut(key, " ")
		if method == r.Method && strings.HasSuffix(path, suffix) {
			if f.failCount[key] > 0 {
				f.failCount[key]--
				if f.failCount[key] == 0 {
					delete(f.failures, key)
				}
			}
			// The body deliberately carries sensitive data to prove it never leaks.
			writeJSON(w, status, map[string]string{"errorMessage": "boom for victim@example.test " + testToken})
			return
		}
	}
	f.route(w, r, path)
}

func (f *fakeKeycloak) route(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch {
	case path == "/users" && r.Method == http.MethodGet:
		f.listUsers(w, r)
	case path == "/users" && r.Method == http.MethodPost:
		f.createUser(w, r)
	case parts[0] == "users" && len(parts) >= 2:
		f.userRoute(w, r, parts)
	case parts[0] == "group-by-path":
		groupPath := "/" + strings.Join(parts[1:], "/")
		id, ok := f.groups[groupPath]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Group path does not exist"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "path": groupPath, "name": parts[len(parts)-1]})
	case parts[0] == "sessions" && len(parts) == 2 && r.Method == http.MethodDelete:
		for _, user := range f.users {
			for i, session := range user.Sessions {
				if session.ID == parts[1] {
					user.Sessions = append(user.Sessions[:i], user.Sessions[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Session not found"})
	case path == "/events" && r.Method == http.MethodGet:
		if f.eventsDenied {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "HTTP 403 Forbidden"})
			return
		}
		user := f.users[r.URL.Query().Get("user")]
		events := []map[string]string{}
		if user != nil && user.LoginEvents > 0 {
			events = append(events, map[string]string{"type": "LOGIN"})
		}
		writeJSON(w, http.StatusOK, events)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unexpected route " + path})
	}
}

func (f *fakeKeycloak) listUsers(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	exact := r.URL.Query().Get("exact") == "true"
	result := []map[string]any{}
	for _, user := range f.users {
		match := strings.EqualFold(user.Email, email)
		if !exact {
			match = strings.Contains(strings.ToLower(user.Email), strings.ToLower(email))
		}
		if match {
			result = append(result, user.representation())
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (u *fakeUser) representation() map[string]any {
	return map[string]any{
		"id": u.ID, "username": u.Email, "email": u.Email, "firstName": u.FirstName, "lastName": u.LastName,
		"enabled": u.Enabled, "emailVerified": u.EmailVerified, "requiredActions": u.RequiredActions,
		"createdTimestamp": 1700000000000,
	}
}

func (f *fakeKeycloak) createUser(w http.ResponseWriter, r *http.Request) {
	var body userRepresentation
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	for _, user := range f.users {
		if strings.EqualFold(user.Email, body.Email) {
			writeJSON(w, http.StatusConflict, map[string]string{"errorMessage": "User exists with same email " + body.Email})
			return
		}
	}
	user := f.addUserLocked(body.Email)
	user.FirstName, user.LastName = body.FirstName, body.LastName
	user.RequiredActions = body.RequiredActions
	user.EmailVerified = body.EmailVerified != nil && *body.EmailVerified
	f.t.Logf("fake: created %s", user.ID)
	w.Header().Set("Location", f.server.URL+"/admin/realms/"+testRealm+"/users/"+user.ID)
	w.WriteHeader(http.StatusCreated)
}

func (f *fakeKeycloak) userRoute(w http.ResponseWriter, r *http.Request, parts []string) {
	user, ok := f.users[parts[1]]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, user.representation())
	case len(parts) == 2 && r.Method == http.MethodDelete:
		delete(f.users, user.ID)
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 3 && parts[2] == "credentials":
		creds := make([]map[string]string, user.Credentials)
		for i := range creds {
			creds[i] = map[string]string{"id": fmt.Sprint(i), "type": "password"}
		}
		writeJSON(w, http.StatusOK, creds)
	case len(parts) == 3 && parts[2] == "federated-identity":
		fed := make([]map[string]string, user.Federated)
		for i := range fed {
			fed[i] = map[string]string{"identityProvider": "google"}
		}
		writeJSON(w, http.StatusOK, fed)
	case len(parts) == 3 && parts[2] == "sessions":
		sessions := []map[string]any{}
		for _, s := range user.Sessions {
			sessions = append(sessions, map[string]any{"id": s.ID, "clients": s.Clients})
		}
		writeJSON(w, http.StatusOK, sessions)
	case len(parts) == 3 && parts[2] == "groups" && r.Method == http.MethodGet:
		groups := []map[string]string{}
		for id := range user.Groups {
			groups = append(groups, map[string]string{"id": id, "path": f.groupPath(id)})
		}
		writeJSON(w, http.StatusOK, groups)
	case len(parts) == 4 && parts[2] == "groups" && r.Method == http.MethodPut:
		user.Groups[parts[3]] = true
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 4 && parts[2] == "groups" && r.Method == http.MethodDelete:
		delete(user.Groups, parts[3])
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 3 && parts[2] == "execute-actions-email" && r.Method == http.MethodPut:
		if f.smtpBroken {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"errorMessage": "Failed to send execute actions email"})
			return
		}
		f.lastEmail.query = map[string]string{}
		for key, values := range r.URL.Query() {
			f.lastEmail.query[key] = values[0]
		}
		_ = json.NewDecoder(r.Body).Decode(&f.lastEmail.actions)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unexpected user route"})
	}
}
