package migrationident

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/snapetech/iptvtunerr/internal/plex"
)

func TestBuildFromPlexAPI(t *testing.T) {
	oldListUsers := plexListUsers
	oldIdentity := plexGetServerIdentity
	oldShared := plexListSharedServers
	defer func() {
		plexListUsers = oldListUsers
		plexGetServerIdentity = oldIdentity
		plexListSharedServers = oldShared
	}()

	plexListUsers = func(string) ([]plex.UserInfo, error) {
		return []plex.UserInfo{
			{ID: 10, UUID: "u-10", Username: "alice", Title: "Alice", Email: "alice@example.com", Home: true},
			{ID: 11, UUID: "u-11", Title: "Kids Room", Managed: true, Restricted: true},
		}, nil
	}
	plexGetServerIdentity = func(string, string) (map[string]string, error) {
		return map[string]string{"machine_identifier": "machine-1"}, nil
	}
	plexListSharedServers = func(string, string) ([]plex.SharedServer, error) {
		return []plex.SharedServer{{ID: 200, UserID: 10, AllowTuners: 1, AllowSync: 1, AllLibraries: true}}, nil
	}

	bundle, err := BuildFromPlexAPI("http://plex:32400", "token")
	if err != nil {
		t.Fatalf("BuildFromPlexAPI: %v", err)
	}
	if bundle.MachineID != "machine-1" {
		t.Fatalf("machine id=%q", bundle.MachineID)
	}
	if len(bundle.Users) != 2 {
		t.Fatalf("users=%+v", bundle.Users)
	}
	if bundle.Users[0].DesiredUsername != "alice" || !bundle.Users[0].AllowTuners || !bundle.Users[0].ServerShared {
		t.Fatalf("user0=%+v", bundle.Users[0])
	}
	if bundle.Users[1].DesiredUsername != "Kids Room" || !bundle.Users[1].Managed {
		t.Fatalf("user1=%+v", bundle.Users[1])
	}
}

func TestBuildPlan(t *testing.T) {
	plan, err := BuildPlan(Bundle{
		Source: "plex_users_api",
		Users: []BundleUser{
			{PlexID: 10, Username: "alice", DesiredUsername: "alice", AllowTuners: true},
			{PlexID: 11, Title: "Kids Room", DesiredUsername: "Kids Room", Managed: true},
		},
	}, "jellyfin", "http://jellyfin:8096")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Target != "jellyfin" || len(plan.Users) != 2 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Users[0].DesiredPolicy.EnableLiveTvAccess == nil || !*plan.Users[0].DesiredPolicy.EnableLiveTvAccess {
		t.Fatalf("policy=%+v", plan.Users[0].DesiredPolicy)
	}
}

func TestBuildOIDCPlan(t *testing.T) {
	plan, err := BuildOIDCPlan(Bundle{
		Source: "plex_users_api",
		Users: []BundleUser{
			{
				PlexID:          10,
				PlexUUID:        "u-10",
				Username:        "alice",
				Title:           "Alice",
				Email:           "alice@example.com",
				DesiredUsername: "alice",
				Home:            true,
				ServerShared:    true,
				AllowTuners:     true,
				AllowSync:       true,
			},
		},
	}, "https://id.example.com", "iptvtunerr")
	if err != nil {
		t.Fatalf("BuildOIDCPlan: %v", err)
	}
	if plan.Issuer != "https://id.example.com" || plan.ClientID != "iptvtunerr" || len(plan.Users) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	user := plan.Users[0]
	if user.SubjectHint != "plex:u-10" || user.PreferredUsername != "alice" {
		t.Fatalf("user=%+v", user)
	}
	for _, want := range []string{"tunerr:migrated", "tunerr:plex-home", "tunerr:plex-shared", "tunerr:live-tv", "tunerr:sync"} {
		if !strings.Contains(strings.Join(user.Groups, ","), want) {
			t.Fatalf("groups=%v missing %q", user.Groups, want)
		}
	}
}

func TestDiffAndApplyKeycloakOIDCPlan(t *testing.T) {
	var createdUsers []string
	var createdGroups []string
	var memberships []string
	var resetPasswords []string
	var executeEmails []string
	var createdUserBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			users := []map[string]any{{"id": "u1", "username": "alice"}}
			for _, name := range createdUsers {
				users = append(users, map[string]any{"id": "u2", "username": name})
			}
			_ = json.NewEncoder(w).Encode(users)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u1", "username": "alice", "enabled": true, "email": "old@example.com"})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u2":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u2", "username": "bob", "enabled": true, "email": "bob@example.com"})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			groups := []map[string]any{{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"}}
			for i, name := range createdGroups {
				groups = append(groups, map[string]any{"id": "g" + string(rune('2'+i)), "name": name, "path": "/" + name})
			}
			_ = json.NewEncoder(w).Encode(groups)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"}})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/users":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode user: %v", err)
			}
			createdUserBodies = append(createdUserBodies, body)
			createdUsers = append(createdUsers, body["username"].(string))
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && (r.URL.Path == "/admin/realms/master/users/u1" || r.URL.Path == "/admin/realms/master/users/u2"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/master/groups":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode group: %v", err)
			}
			createdGroups = append(createdGroups, body["name"].(string))
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/groups/"):
			memberships = append(memberships, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/reset-password"):
			resetPasswords = append(resetPasswords, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
			executeEmails = append(executeEmails, r.URL.Path+"?"+r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u2/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	plan := OIDCPlan{
		Users: []OIDCPlanUser{
			{SubjectHint: "plex:u1", PreferredUsername: "alice", Groups: []string{"tunerr:migrated", "tunerr:live-tv"}},
			{SubjectHint: "plex:u2", PreferredUsername: "bob", Email: "bob@example.com", Groups: []string{"tunerr:migrated", "tunerr:sync"}},
		},
	}
	diff, err := DiffKeycloakOIDCPlan(plan, srv.URL, "master", "token")
	if err != nil {
		t.Fatalf("DiffKeycloakOIDCPlan: %v", err)
	}
	if diff.CreateUserCount != 1 || diff.CreateGroupCount != 2 || diff.AddMembershipCount != 3 {
		t.Fatalf("diff=%+v", diff)
	}
	apply, err := ApplyKeycloakOIDCPlanWithOptions(plan, srv.URL, "master", "token", KeycloakApplyOptions{
		BootstrapPassword: "Temp123!",
		PasswordTemporary: true,
		EmailActions:      []string{"UPDATE_PASSWORD"},
		EmailClientID:     "iptvtunerr",
	})
	if err != nil {
		t.Fatalf("ApplyKeycloakOIDCPlan: %v", err)
	}
	if len(createdUsers) != 1 || createdUsers[0] != "bob" {
		t.Fatalf("createdUsers=%v", createdUsers)
	}
	if attrs, ok := createdUserBodies[0]["attributes"].(map[string]any); !ok || attrs["tunerr_subject_hint"] == nil {
		t.Fatalf("createdUserBodies=%v", createdUserBodies)
	}
	if apply.CreateUserCount != 1 || apply.AddMembershipCount == 0 {
		t.Fatalf("apply=%+v", apply)
	}
	if len(createdGroups) == 0 || len(memberships) == 0 {
		t.Fatalf("createdGroups=%v memberships=%v", createdGroups, memberships)
	}
	if len(resetPasswords) != 2 {
		t.Fatalf("resetPasswords=%v", resetPasswords)
	}
	if len(executeEmails) != 1 || !strings.Contains(executeEmails[0], "client_id=iptvtunerr") {
		t.Fatalf("executeEmails=%v", executeEmails)
	}
	if apply.MetadataUpdateCount != 1 {
		t.Fatalf("apply=%+v", apply)
	}
}

func TestDiffKeycloakOIDCPlanWithCredentialAuth(t *testing.T) {
	var sawTokenGrant bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/realms/master/protocol/openid-connect/token":
			sawTokenGrant = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				t.Fatalf("form=%v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			if auth := r.Header.Get("Authorization"); auth != "Bearer fresh-token" {
				t.Fatalf("auth=%q", auth)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			if auth := r.Header.Get("Authorization"); auth != "Bearer fresh-token" {
				t.Fatalf("auth=%q", auth)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	plan := OIDCPlan{
		Users: []OIDCPlanUser{{SubjectHint: "plex:u1", PreferredUsername: "alice", Groups: []string{"tunerr:migrated"}}},
	}
	diff, err := DiffKeycloakOIDCPlanWithAuth(plan, srv.URL, "master", "", "admin", "secret")
	if err != nil {
		t.Fatalf("DiffKeycloakOIDCPlanWithAuth: %v", err)
	}
	if !sawTokenGrant || diff.CreateUserCount != 1 {
		t.Fatalf("sawTokenGrant=%v diff=%+v", sawTokenGrant, diff)
	}
}

func TestDiffAndApplyAuthentikOIDCPlan(t *testing.T) {
	var createdUsers []map[string]any
	var createdGroups []string
	var memberships []string
	var setPasswords []string
	var recoveryEmails []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"pk": 1, "username": "alice", "groups": []any{"g1"}}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/1/":
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 1, "username": "alice", "name": "Old Alice", "groups": []any{"g1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/2/":
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 2, "username": "bob", "groups": []any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/groups/":
			groups := []map[string]any{{"pk": "g1", "name": "tunerr:migrated", "users": []any{1}}}
			for i, name := range createdGroups {
				groups = append(groups, map[string]any{"pk": "g-new-" + string(rune('1'+i)), "name": name, "users": []any{}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": groups})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/users/":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode user: %v", err)
			}
			createdUsers = append(createdUsers, body)
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 2, "username": body["username"]})
		case r.Method == http.MethodPatch && (r.URL.Path == "/api/v3/core/users/1/" || r.URL.Path == "/api/v3/core/users/2/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/core/groups/":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode group: %v", err)
			}
			createdGroups = append(createdGroups, body["name"].(string))
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": "g-new", "name": body["name"]})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/add_user/"):
			memberships = append(memberships, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/set_password/"):
			setPasswords = append(setPasswords, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/recovery_email/"):
			recoveryEmails = append(recoveryEmails, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	plan := OIDCPlan{
		Users: []OIDCPlanUser{
			{SubjectHint: "plex:u1", PreferredUsername: "alice", Groups: []string{"tunerr:migrated", "tunerr:live-tv"}},
			{SubjectHint: "plex:u2", PreferredUsername: "bob", Email: "bob@example.com", PlexUUID: "u2", Groups: []string{"tunerr:migrated", "tunerr:sync"}},
		},
	}
	diff, err := DiffAuthentikOIDCPlan(plan, srv.URL, "token")
	if err != nil {
		t.Fatalf("DiffAuthentikOIDCPlan: %v", err)
	}
	if diff.CreateUserCount != 1 || diff.CreateGroupCount != 2 || diff.AddMembershipCount != 3 {
		t.Fatalf("diff=%+v", diff)
	}
	apply, err := ApplyAuthentikOIDCPlanWithOptions(plan, srv.URL, "token", AuthentikApplyOptions{
		BootstrapPassword: "Temp123!",
		RecoveryEmail:     true,
	})
	if err != nil {
		t.Fatalf("ApplyAuthentikOIDCPlanWithOptions: %v", err)
	}
	if len(createdUsers) != 1 || createdUsers[0]["username"] != "bob" {
		t.Fatalf("createdUsers=%v", createdUsers)
	}
	if attrs, ok := createdUsers[0]["attributes"].(map[string]any); !ok || attrs["tunerr_plex_uuid"] != "u2" {
		t.Fatalf("createdUsers=%v", createdUsers)
	}
	if apply.CreateUserCount != 1 || apply.AddMembershipCount == 0 {
		t.Fatalf("apply=%+v", apply)
	}
	if len(createdGroups) == 0 || len(memberships) == 0 {
		t.Fatalf("createdGroups=%v memberships=%v", createdGroups, memberships)
	}
	if len(setPasswords) != 2 || len(recoveryEmails) != 1 {
		t.Fatalf("setPasswords=%v recoveryEmails=%v", setPasswords, recoveryEmails)
	}
	if apply.MetadataUpdateCount != 2 {
		t.Fatalf("apply=%+v", apply)
	}
}

func TestDiffPlanAndApplyPlan(t *testing.T) {
	var created []string
	var policyUpdates []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			users := []map[string]any{{"Id": "u-1", "Name": "alice", "Policy": map[string]any{"EnableLiveTvAccess": false, "EnableRemoteAccess": false}, "HasConfiguredPassword": true}}
			for i, name := range created {
				users = append(users, map[string]any{"Id": "u-new-" + string(rune('1'+i)), "Name": name, "Policy": map[string]any{"EnableLiveTvAccess": false}})
			}
			_ = json.NewEncoder(w).Encode(users)
		case r.Method == http.MethodGet && r.URL.Path == "/Users/u-2":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "u-2", "Name": "kids", "Policy": map[string]any{"EnableLiveTvAccess": false}})
		case r.Method == http.MethodPost && r.URL.Path == "/Users/New":
			name := r.URL.Query().Get("Name")
			created = append(created, name)
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "u-2", "Name": name})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/Policy"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode policy: %v", err)
			}
			policyUpdates = append(policyUpdates, body)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	plan := Plan{
		Target: "emby",
		Users: []PlanUser{
			{PlexID: 10, DesiredUsername: "alice", DesiredPolicy: DesiredPolicy{EnableLiveTvAccess: boolPtr(true), EnableRemoteAccess: boolPtr(true)}},
			{PlexID: 11, DesiredUsername: "kids", DesiredPolicy: DesiredPolicy{EnableLiveTvAccess: boolPtr(true)}},
		},
	}
	diff, err := DiffPlan(plan, srv.URL, "token")
	if err != nil {
		t.Fatalf("DiffPlan: %v", err)
	}
	if diff.ReuseCount != 1 || diff.CreateCount != 1 || diff.PolicyUpdateCount != 1 {
		t.Fatalf("diff=%+v", diff)
	}
	if !diff.Entries[0].PolicyUpdate || len(diff.Entries[0].PolicyDrift) != 2 {
		t.Fatalf("entry=%+v", diff.Entries[0])
	}
	res, err := ApplyPlan(plan, srv.URL, "token")
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if len(created) != 1 || created[0] != "kids" {
		t.Fatalf("created=%v", created)
	}
	if len(res.Users) != 2 || !res.Users[1].Created {
		t.Fatalf("result=%+v", res)
	}
	if res.PolicyUpdateCount != 2 || len(policyUpdates) != 2 {
		t.Fatalf("policyUpdates=%v result=%+v", policyUpdates, res)
	}
	if !res.Users[1].ActivationPending || res.ActivationPendingCount != 1 {
		t.Fatalf("result=%+v", res)
	}
}

func TestBuildRolloutPlan(t *testing.T) {
	rollout, err := BuildRolloutPlan(Bundle{
		Source: "plex_users_api",
		Users:  []BundleUser{{PlexID: 10, Username: "alice", DesiredUsername: "alice"}},
	}, []TargetSpec{{Target: "emby", Host: "http://emby:8096"}, {Target: "jellyfin", Host: "http://jellyfin:8096"}})
	if err != nil {
		t.Fatalf("BuildRolloutPlan: %v", err)
	}
	if len(rollout.Plans) != 2 {
		t.Fatalf("plans=%+v", rollout.Plans)
	}
}

func TestAuditBundleTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "u-1", "Name": "alice", "Policy": map[string]any{"EnableLiveTvAccess": false}},
		})
	}))
	defer srv.Close()

	bundle := Bundle{
		Source: "plex_users_api",
		Users: []BundleUser{
			{PlexID: 10, Username: "alice", DesiredUsername: "alice", AllowTuners: true},
			{PlexID: 11, Title: "Kids", DesiredUsername: "Kids", Managed: true, ServerShared: true, AllowTuners: true},
		},
	}
	result, err := AuditBundleTargets(bundle, []TargetSpec{{Target: "emby", Host: srv.URL}}, map[string]ApplySpec{
		"emby": {Host: srv.URL, Token: "token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	if result.Status != "ready_to_apply" || !result.ReadyToApply {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results=%+v", result.Results)
	}
	target := result.Results[0]
	if target.CreateCount != 1 || target.ReuseCount != 1 || target.PolicyUpdateCount != 1 || target.ActivationPendingCount != 1 {
		t.Fatalf("target=%+v", target)
	}
	if target.ManualFollowUpCount != 1 || len(target.MissingUsers) != 1 || target.MissingUsers[0] != "Kids" {
		t.Fatalf("target=%+v", target)
	}
	if len(target.PolicyUpdateUsers) != 1 || target.PolicyUpdateUsers[0] != "alice" {
		t.Fatalf("target=%+v", target)
	}
	if len(target.ActivationPendingUsers) != 1 || target.ActivationPendingUsers[0] != "alice" {
		t.Fatalf("target=%+v", target)
	}
}

func TestFormatAuditSummary(t *testing.T) {
	text := FormatAuditSummary(AuditResult{
		Status:       "ready_to_apply",
		ReadyToApply: true,
		TargetCount:  1,
		Results: []TargetAudit{{
			Target:                 "emby",
			TargetHost:             "http://emby:8096",
			Status:                 "ready_to_apply",
			StatusReason:           "some Plex users still need destination accounts",
			CreateCount:            2,
			ReuseCount:             3,
			PolicyUpdateCount:      1,
			ActivationPendingCount: 1,
			ManualFollowUpCount:    1,
			MissingUsers:           []string{"Kids"},
			PolicyUpdateUsers:      []string{"alice"},
			ActivationPendingUsers: []string{"alice"},
			ManualFollowUpUsers:    []AuditUser{{DesiredUsername: "Kids"}},
		}},
	})
	for _, want := range []string{
		"Identity migration audit",
		"overall_status: ready_to_apply",
		"[emby] http://emby:8096",
		"missing_users: Kids",
		"policy_update_users: alice",
		"activation_pending_users: alice",
		"manual_follow_up_users: Kids",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
}

func TestAuditOIDCPlanTargets(t *testing.T) {
	keycloakSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "u1", "username": "alice", "enabled": true},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "u1", "username": "alice", "enabled": true,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/admin/realms/master/users/u1/groups":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "g1", "name": "tunerr:migrated", "path": "/tunerr:migrated"},
			})
		default:
			t.Fatalf("unexpected keycloak request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer keycloakSrv.Close()

	authentikSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"pk": 1, "username": "alice", "groups": []any{"g1"}}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/users/1/":
			_ = json.NewEncoder(w).Encode(map[string]any{"pk": 1, "username": "alice", "groups": []any{"g1"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/groups/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{"pk": "g1", "name": "tunerr:migrated", "users": []any{1}}},
			})
		default:
			t.Fatalf("unexpected authentik request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer authentikSrv.Close()

	plan := OIDCPlan{
		Issuer:   "https://id.example.com",
		ClientID: "iptvtunerr",
		Users: []OIDCPlanUser{
			{SubjectHint: "plex:u1", PreferredUsername: "alice", Groups: []string{"tunerr:migrated", "tunerr:live-tv"}},
			{SubjectHint: "plex:u2", PreferredUsername: "bob", Groups: []string{"tunerr:migrated", "tunerr:sync"}},
		},
	}
	result, err := AuditOIDCPlanTargets(plan, []OIDCTargetSpec{
		{Target: "keycloak", Host: keycloakSrv.URL, Realm: "master"},
		{Target: "authentik", Host: authentikSrv.URL},
	}, map[string]OIDCApplySpec{
		"keycloak":  {Host: keycloakSrv.URL, Realm: "master", Token: "token"},
		"authentik": {Host: authentikSrv.URL, Token: "token"},
	})
	if err != nil {
		t.Fatalf("AuditOIDCPlanTargets: %v", err)
	}
	if result.Status != "ready_to_apply" || !result.ReadyToApply || result.TargetCount != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Results[0].CreateUserCount == 0 && result.Results[1].CreateUserCount == 0 {
		t.Fatalf("result=%+v", result)
	}
	text := FormatOIDCAuditSummary(*result)
	for _, want := range []string{
		"OIDC migration audit",
		"issuer: https://id.example.com",
		"[keycloak]",
		"[authentik]",
		"missing_users: bob",
		"metadata_update_count: 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
}

func TestAuditBundleTargetsActivationPendingKeepsReadyToApply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Users" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "u-1", "Name": "alice", "Policy": map[string]any{"EnableLiveTvAccess": true}},
		})
	}))
	defer srv.Close()

	bundle := Bundle{
		Source: "plex_users_api",
		Users: []BundleUser{
			{PlexID: 10, Username: "alice", DesiredUsername: "alice", AllowTuners: true},
		},
	}
	result, err := AuditBundleTargets(bundle, []TargetSpec{{Target: "emby", Host: srv.URL}}, map[string]ApplySpec{
		"emby": {Host: srv.URL, Token: "token"},
	})
	if err != nil {
		t.Fatalf("AuditBundleTargets: %v", err)
	}
	target := result.Results[0]
	if target.Status != "ready_to_apply" {
		t.Fatalf("target=%+v", target)
	}
	if !strings.Contains(target.StatusReason, "activation-ready") {
		t.Fatalf("target=%+v", target)
	}
	if len(target.ActivationPendingUsers) != 1 || target.ActivationPendingUsers[0] != "alice" {
		t.Fatalf("target=%+v", target)
	}
}
