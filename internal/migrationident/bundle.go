package migrationident

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/authentik"
	"github.com/snapetech/iptvtunerr/internal/emby"
	"github.com/snapetech/iptvtunerr/internal/keycloak"
	"github.com/snapetech/iptvtunerr/internal/plex"
)

type Bundle struct {
	GeneratedAt string       `json:"generated_at"`
	Source      string       `json:"source"`
	MachineID   string       `json:"machine_id,omitempty"`
	Users       []BundleUser `json:"users"`
	Notes       []string     `json:"notes,omitempty"`
}

type BundleUser struct {
	PlexID          int    `json:"plex_id"`
	PlexUUID        string `json:"plex_uuid,omitempty"`
	Username        string `json:"username,omitempty"`
	Title           string `json:"title,omitempty"`
	Email           string `json:"email,omitempty"`
	Home            bool   `json:"home"`
	Managed         bool   `json:"managed"`
	Restricted      bool   `json:"restricted"`
	ServerShared    bool   `json:"server_shared"`
	SharedServerID  int    `json:"shared_server_id,omitempty"`
	AllowTuners     bool   `json:"allow_tuners"`
	AllowSync       bool   `json:"allow_sync"`
	AllLibraries    bool   `json:"all_libraries"`
	DesiredUsername string `json:"desired_username,omitempty"`
}

type Plan struct {
	GeneratedAt  string     `json:"generated_at"`
	Target       string     `json:"target"`
	BundleSource string     `json:"bundle_source"`
	TargetHost   string     `json:"target_host,omitempty"`
	Users        []PlanUser `json:"users"`
	Notes        []string   `json:"notes,omitempty"`
}

type OIDCPlan struct {
	GeneratedAt  string         `json:"generated_at"`
	BundleSource string         `json:"bundle_source"`
	Issuer       string         `json:"issuer,omitempty"`
	ClientID     string         `json:"client_id,omitempty"`
	Users        []OIDCPlanUser `json:"users"`
	Notes        []string       `json:"notes,omitempty"`
}

type OIDCPlanUser struct {
	PlexID            int      `json:"plex_id"`
	PlexUUID          string   `json:"plex_uuid,omitempty"`
	SubjectHint       string   `json:"subject_hint"`
	PreferredUsername string   `json:"preferred_username"`
	DisplayName       string   `json:"display_name,omitempty"`
	Email             string   `json:"email,omitempty"`
	Groups            []string `json:"groups,omitempty"`
}

type KeycloakDiffEntry struct {
	SubjectHint       string   `json:"subject_hint"`
	PreferredUsername string   `json:"preferred_username"`
	Status            string   `json:"status"`
	ExistingID        string   `json:"existing_id,omitempty"`
	MissingGroups     []string `json:"missing_groups,omitempty"`
	MetadataUpdate    bool     `json:"metadata_update,omitempty"`
	MetadataDrift     []string `json:"metadata_drift,omitempty"`
}

type KeycloakDiffResult struct {
	ComparedAt             string              `json:"compared_at"`
	Host                   string              `json:"host"`
	Realm                  string              `json:"realm"`
	CreateUserCount        int                 `json:"create_user_count"`
	CreateGroupCount       int                 `json:"create_group_count"`
	AddMembershipCount     int                 `json:"add_membership_count"`
	MetadataUpdateCount    int                 `json:"metadata_update_count"`
	ActivationPendingCount int                 `json:"activation_pending_count"`
	Entries                []KeycloakDiffEntry `json:"entries"`
	Notes                  []string            `json:"notes,omitempty"`
}

type KeycloakApplyResult struct {
	AppliedAt              string              `json:"applied_at"`
	Host                   string              `json:"host"`
	Realm                  string              `json:"realm"`
	CreateUserCount        int                 `json:"create_user_count"`
	CreateGroupCount       int                 `json:"create_group_count"`
	AddMembershipCount     int                 `json:"add_membership_count"`
	MetadataUpdateCount    int                 `json:"metadata_update_count"`
	ActivationPendingCount int                 `json:"activation_pending_count"`
	Entries                []KeycloakDiffEntry `json:"entries"`
	Notes                  []string            `json:"notes,omitempty"`
}

type KeycloakApplyOptions struct {
	BootstrapPassword string   `json:"bootstrap_password,omitempty"`
	PasswordTemporary bool     `json:"password_temporary,omitempty"`
	EmailActions      []string `json:"email_actions,omitempty"`
	EmailClientID     string   `json:"email_client_id,omitempty"`
	EmailRedirectURI  string   `json:"email_redirect_uri,omitempty"`
	EmailLifespanSec  int      `json:"email_lifespan_sec,omitempty"`
}

type AuthentikDiffEntry struct {
	SubjectHint       string   `json:"subject_hint"`
	PreferredUsername string   `json:"preferred_username"`
	Status            string   `json:"status"`
	ExistingID        string   `json:"existing_id,omitempty"`
	MissingGroups     []string `json:"missing_groups,omitempty"`
	MetadataUpdate    bool     `json:"metadata_update,omitempty"`
	MetadataDrift     []string `json:"metadata_drift,omitempty"`
}

type AuthentikDiffResult struct {
	ComparedAt             string               `json:"compared_at"`
	Host                   string               `json:"host"`
	CreateUserCount        int                  `json:"create_user_count"`
	CreateGroupCount       int                  `json:"create_group_count"`
	AddMembershipCount     int                  `json:"add_membership_count"`
	MetadataUpdateCount    int                  `json:"metadata_update_count"`
	ActivationPendingCount int                  `json:"activation_pending_count"`
	Entries                []AuthentikDiffEntry `json:"entries"`
	Notes                  []string             `json:"notes,omitempty"`
}

type AuthentikApplyResult struct {
	AppliedAt              string               `json:"applied_at"`
	Host                   string               `json:"host"`
	CreateUserCount        int                  `json:"create_user_count"`
	CreateGroupCount       int                  `json:"create_group_count"`
	AddMembershipCount     int                  `json:"add_membership_count"`
	MetadataUpdateCount    int                  `json:"metadata_update_count"`
	ActivationPendingCount int                  `json:"activation_pending_count"`
	Entries                []AuthentikDiffEntry `json:"entries"`
	Notes                  []string             `json:"notes,omitempty"`
}

type AuthentikApplyOptions struct {
	BootstrapPassword string `json:"bootstrap_password,omitempty"`
	RecoveryEmail     bool   `json:"recovery_email,omitempty"`
}

type OIDCTargetSpec struct {
	Target string
	Host   string
	Realm  string
}

type OIDCApplySpec struct {
	Host     string
	Realm    string
	Token    string
	Username string
	Password string
}

type OIDCTargetAudit struct {
	Target                 string   `json:"target"`
	TargetHost             string   `json:"target_host"`
	Status                 string   `json:"status"`
	StatusReason           string   `json:"status_reason,omitempty"`
	ReadyToApply           bool     `json:"ready_to_apply"`
	CreateUserCount        int      `json:"create_user_count"`
	CreateGroupCount       int      `json:"create_group_count"`
	AddMembershipCount     int      `json:"add_membership_count"`
	MetadataUpdateCount    int      `json:"metadata_update_count"`
	ActivationPendingCount int      `json:"activation_pending_count"`
	MissingUsers           []string `json:"missing_users,omitempty"`
	MissingGroups          []string `json:"missing_groups,omitempty"`
	MembershipUsers        []string `json:"membership_users,omitempty"`
	MetadataUsers          []string `json:"metadata_users,omitempty"`
}

type OIDCAuditResult struct {
	ComparedAt       string            `json:"compared_at"`
	Issuer           string            `json:"issuer,omitempty"`
	ClientID         string            `json:"client_id,omitempty"`
	Status           string            `json:"status"`
	ReadyToApply     bool              `json:"ready_to_apply"`
	TargetCount      int               `json:"target_count"`
	ReadyTargetCount int               `json:"ready_target_count"`
	Results          []OIDCTargetAudit `json:"results"`
	Notes            []string          `json:"notes,omitempty"`
}

type DesiredPolicy struct {
	EnableLiveTvAccess       *bool `json:"enable_live_tv_access,omitempty"`
	EnableRemoteAccess       *bool `json:"enable_remote_access,omitempty"`
	EnableContentDownloading *bool `json:"enable_content_downloading,omitempty"`
	EnableSyncTranscoding    *bool `json:"enable_sync_transcoding,omitempty"`
	EnableAllFolders         *bool `json:"enable_all_folders,omitempty"`
}

type PlanUser struct {
	PlexID          int           `json:"plex_id"`
	Username        string        `json:"username,omitempty"`
	Title           string        `json:"title,omitempty"`
	Email           string        `json:"email,omitempty"`
	Home            bool          `json:"home"`
	Managed         bool          `json:"managed"`
	ServerShared    bool          `json:"server_shared"`
	AllowTuners     bool          `json:"allow_tuners"`
	AllowSync       bool          `json:"allow_sync"`
	AllLibraries    bool          `json:"all_libraries"`
	DesiredUsername string        `json:"desired_username"`
	DesiredPolicy   DesiredPolicy `json:"desired_policy,omitempty"`
}

type DiffEntry struct {
	PlexID            int      `json:"plex_id"`
	DesiredUsername   string   `json:"desired_username"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	ExistingID        string   `json:"existing_id,omitempty"`
	PolicyUpdate      bool     `json:"policy_update,omitempty"`
	PolicyDrift       []string `json:"policy_drift,omitempty"`
	ActivationPending bool     `json:"activation_pending,omitempty"`
	ActivationReason  string   `json:"activation_reason,omitempty"`
}

type DiffResult struct {
	ComparedAt             string      `json:"compared_at"`
	Target                 string      `json:"target"`
	TargetHost             string      `json:"target_host"`
	CreateCount            int         `json:"create_count"`
	ReuseCount             int         `json:"reuse_count"`
	ConflictCount          int         `json:"conflict_count"`
	PolicyUpdateCount      int         `json:"policy_update_count"`
	ActivationPendingCount int         `json:"activation_pending_count"`
	Entries                []DiffEntry `json:"entries"`
	Notes                  []string    `json:"notes,omitempty"`
}

type ApplyUserResult struct {
	PlexID            int      `json:"plex_id"`
	DesiredUsername   string   `json:"desired_username"`
	ID                string   `json:"id,omitempty"`
	Created           bool     `json:"created"`
	PolicyUpdated     bool     `json:"policy_updated,omitempty"`
	PolicyDrift       []string `json:"policy_drift,omitempty"`
	ActivationPending bool     `json:"activation_pending,omitempty"`
	ActivationReason  string   `json:"activation_reason,omitempty"`
}

type ApplyResult struct {
	AppliedAt              string            `json:"applied_at"`
	Target                 string            `json:"target"`
	TargetHost             string            `json:"target_host"`
	PolicyUpdateCount      int               `json:"policy_update_count"`
	ActivationPendingCount int               `json:"activation_pending_count"`
	Users                  []ApplyUserResult `json:"users"`
	Notes                  []string          `json:"notes,omitempty"`
}

type TargetSpec struct {
	Target string
	Host   string
}

type ApplySpec struct {
	Host  string
	Token string
}

type RolloutPlan struct {
	GeneratedAt  string   `json:"generated_at"`
	BundleSource string   `json:"bundle_source"`
	Plans        []Plan   `json:"plans"`
	Notes        []string `json:"notes,omitempty"`
}

type RolloutDiffResult struct {
	ComparedAt string       `json:"compared_at"`
	Results    []DiffResult `json:"results"`
	Notes      []string     `json:"notes,omitempty"`
}

type RolloutApplyResult struct {
	AppliedAt string        `json:"applied_at"`
	Results   []ApplyResult `json:"results"`
	Notes     []string      `json:"notes,omitempty"`
}

type AuditUser struct {
	PlexID          int    `json:"plex_id"`
	DesiredUsername string `json:"desired_username"`
	Managed         bool   `json:"managed"`
	ServerShared    bool   `json:"server_shared"`
	AllowTuners     bool   `json:"allow_tuners"`
	AllowSync       bool   `json:"allow_sync"`
	AllLibraries    bool   `json:"all_libraries"`
}

type TargetAudit struct {
	Target                 string      `json:"target"`
	TargetHost             string      `json:"target_host"`
	Status                 string      `json:"status"`
	StatusReason           string      `json:"status_reason,omitempty"`
	ReadyToApply           bool        `json:"ready_to_apply"`
	CreateCount            int         `json:"create_count"`
	ReuseCount             int         `json:"reuse_count"`
	ConflictCount          int         `json:"conflict_count"`
	PolicyUpdateCount      int         `json:"policy_update_count"`
	ActivationPendingCount int         `json:"activation_pending_count"`
	SharedUserCount        int         `json:"shared_user_count"`
	ManagedUserCount       int         `json:"managed_user_count"`
	TunerEntitledCount     int         `json:"tuner_entitled_count"`
	ManualFollowUpCount    int         `json:"manual_follow_up_count"`
	MissingUsers           []string    `json:"missing_users,omitempty"`
	PolicyUpdateUsers      []string    `json:"policy_update_users,omitempty"`
	ActivationPendingUsers []string    `json:"activation_pending_users,omitempty"`
	ManualFollowUpUsers    []AuditUser `json:"manual_follow_up_users,omitempty"`
	Diff                   DiffResult  `json:"diff"`
}

type AuditResult struct {
	ComparedAt       string        `json:"compared_at"`
	Source           string        `json:"source,omitempty"`
	Status           string        `json:"status"`
	ReadyToApply     bool          `json:"ready_to_apply"`
	TargetCount      int           `json:"target_count"`
	ReadyTargetCount int           `json:"ready_target_count"`
	ConflictCount    int           `json:"conflict_count"`
	Results          []TargetAudit `json:"results"`
	Notes            []string      `json:"notes,omitempty"`
}

var (
	plexListUsers         = plex.ListUsers
	plexGetServerIdentity = plex.GetServerIdentity
	plexListSharedServers = plex.ListSharedServers
)

func BuildFromPlexAPI(plexBaseURL, plexToken string) (*Bundle, error) {
	plexBaseURL = strings.TrimSpace(plexBaseURL)
	plexToken = strings.TrimSpace(plexToken)
	if plexBaseURL == "" {
		return nil, fmt.Errorf("plex base url required")
	}
	if plexToken == "" {
		return nil, fmt.Errorf("plex token required")
	}
	users, err := plexListUsers(plexToken)
	if err != nil {
		return nil, fmt.Errorf("list plex users: %w", err)
	}
	identity, err := plexGetServerIdentity(plexBaseURL, plexToken)
	if err != nil {
		return nil, fmt.Errorf("get plex server identity: %w", err)
	}
	machineID := strings.TrimSpace(identity["machine_identifier"])
	sharedByUser := map[int]plex.SharedServer{}
	if machineID != "" {
		if shared, err := plexListSharedServers(plexToken, machineID); err == nil {
			for _, item := range shared {
				sharedByUser[item.UserID] = item
			}
		}
	}
	outUsers := make([]BundleUser, 0, len(users))
	for _, user := range users {
		shared := sharedByUser[user.ID]
		outUsers = append(outUsers, BundleUser{
			PlexID:          user.ID,
			PlexUUID:        strings.TrimSpace(user.UUID),
			Username:        strings.TrimSpace(user.Username),
			Title:           strings.TrimSpace(user.Title),
			Email:           strings.TrimSpace(user.Email),
			Home:            user.Home,
			Managed:         user.Managed,
			Restricted:      user.Restricted,
			ServerShared:    shared.UserID != 0,
			SharedServerID:  shared.ID,
			AllowTuners:     shared.AllowTuners > 0,
			AllowSync:       shared.AllowSync > 0,
			AllLibraries:    shared.AllLibraries,
			DesiredUsername: desiredUsername(user.ID, user.Username, user.Title, user.Email),
		})
	}
	return &Bundle{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      "plex_users_api",
		MachineID:   machineID,
		Users:       outUsers,
		Notes: []string{
			"Identity bundle exports Plex users plus server-share tuner hints when they are visible from plex.tv.",
			"Passwords are not exported; destination onboarding/reset remains separate.",
			"OIDC/Caddy-style account provisioning is future scope; this slice targets Emby/Jellyfin local users.",
		},
	}, nil
}

func BuildPlan(bundle Bundle, target, host string) (*Plan, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target != "emby" && target != "jellyfin" {
		return nil, fmt.Errorf("target must be emby or jellyfin")
	}
	users := make([]PlanUser, 0, len(bundle.Users))
	for _, user := range bundle.Users {
		name := strings.TrimSpace(firstNonEmpty(user.DesiredUsername, desiredUsername(user.PlexID, user.Username, user.Title, user.Email)))
		if name == "" {
			continue
		}
		users = append(users, PlanUser{
			PlexID:          user.PlexID,
			Username:        strings.TrimSpace(user.Username),
			Title:           strings.TrimSpace(user.Title),
			Email:           strings.TrimSpace(user.Email),
			Home:            user.Home,
			Managed:         user.Managed,
			ServerShared:    user.ServerShared,
			AllowTuners:     user.AllowTuners,
			AllowSync:       user.AllowSync,
			AllLibraries:    user.AllLibraries,
			DesiredUsername: name,
			DesiredPolicy:   desiredPolicyForUser(user),
		})
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("bundle has no users")
	}
	return &Plan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Target:       target,
		BundleSource: strings.TrimSpace(bundle.Source),
		TargetHost:   strings.TrimSpace(host),
		Users:        users,
		Notes: []string{
			"Plan creates or reuses destination local users by stable desired username and carries additive destination policy grants that can be inferred safely from Plex share state.",
			"Folder-specific library permissions, passwords, and SSO/OIDC lifecycle remain follow-on work.",
		},
	}, nil
}

func BuildOIDCPlan(bundle Bundle, issuer, clientID string) (*OIDCPlan, error) {
	users := make([]OIDCPlanUser, 0, len(bundle.Users))
	for _, user := range bundle.Users {
		preferred := strings.TrimSpace(firstNonEmpty(user.DesiredUsername, desiredUsername(user.PlexID, user.Username, user.Title, user.Email)))
		if preferred == "" {
			continue
		}
		users = append(users, OIDCPlanUser{
			PlexID:            user.PlexID,
			PlexUUID:          strings.TrimSpace(user.PlexUUID),
			SubjectHint:       oidcSubjectHint(user),
			PreferredUsername: preferred,
			DisplayName:       strings.TrimSpace(firstNonEmpty(user.Title, user.Username, preferred)),
			Email:             strings.TrimSpace(user.Email),
			Groups:            oidcGroupsForUser(user),
		})
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("bundle has no users")
	}
	return &OIDCPlan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		BundleSource: strings.TrimSpace(bundle.Source),
		Issuer:       strings.TrimSpace(issuer),
		ClientID:     strings.TrimSpace(clientID),
		Users:        users,
		Notes: []string{
			"OIDC plan is provider-agnostic; it derives subject hints, preferred usernames, display names, email hints, and stable Tunerr group claims from the Plex identity bundle.",
			"Use this as the contract for future Authentik/Keycloak/Caddy-backed provisioning instead of binding migration core logic to one provider API.",
		},
	}, nil
}

func DiffKeycloakOIDCPlan(plan OIDCPlan, host, realm, token string) (*KeycloakDiffResult, error) {
	return DiffKeycloakOIDCPlanWithAuth(plan, host, realm, token, "", "")
}

func DiffKeycloakOIDCPlanWithAuth(plan OIDCPlan, host, realm, token, username, password string) (*KeycloakDiffResult, error) {
	cfg, err := keycloakConfig(host, realm, token, username, password)
	if err != nil {
		return nil, err
	}
	users, err := keycloak.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	groups, err := keycloak.ListGroups(cfg)
	if err != nil {
		return nil, err
	}
	groupByName := map[string]string{}
	for _, group := range groups {
		name := strings.TrimSpace(firstNonEmpty(group.Path, group.Name))
		if name != "" {
			groupByName[name] = strings.TrimSpace(group.ID)
		}
		if rawName := strings.TrimSpace(group.Name); rawName != "" {
			groupByName[rawName] = strings.TrimSpace(group.ID)
			groupByName["/"+rawName] = strings.TrimSpace(group.ID)
		}
	}
	result := &KeycloakDiffResult{
		ComparedAt: time.Now().UTC().Format(time.RFC3339),
		Host:       cfg.Host,
		Realm:      cfg.Realm,
		Entries:    make([]KeycloakDiffEntry, 0, len(plan.Users)),
		Notes: []string{
			"Keycloak diff compares the provider-agnostic OIDC plan against live Keycloak users and group membership using preferred_username plus Tunerr-owned group claims.",
		},
	}
	for _, want := range plan.Users {
		entry := KeycloakDiffEntry{
			SubjectHint:       want.SubjectHint,
			PreferredUsername: want.PreferredUsername,
			Status:            "create_user",
		}
		if existing := keycloak.FindUserByUsername(users, want.PreferredUsername); existing != nil {
			entry.Status = "reuse_user"
			entry.ExistingID = strings.TrimSpace(existing.ID)
			fullUser, err := keycloak.GetUser(cfg, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("get keycloak user %q: %w", want.PreferredUsername, err)
			}
			entry.MetadataDrift = keycloakMetadataDrift(*fullUser, want)
			entry.MetadataUpdate = len(entry.MetadataDrift) > 0
			if entry.MetadataUpdate {
				result.MetadataUpdateCount++
			}
			userGroups, err := keycloak.GetUserGroups(cfg, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("get keycloak groups for %q: %w", want.PreferredUsername, err)
			}
			missing := missingKeycloakGroups(want.Groups, userGroups)
			entry.MissingGroups = missing
			if len(missing) > 0 {
				result.AddMembershipCount += len(missing)
			}
			if pending := keycloakActivationPending(*fullUser); pending {
				result.ActivationPendingCount++
			}
		} else {
			result.CreateUserCount++
			if len(want.Groups) > 0 {
				result.AddMembershipCount += len(want.Groups)
				result.ActivationPendingCount++
			}
		}
		for _, group := range want.Groups {
			if _, ok := groupByName[group]; !ok {
				result.CreateGroupCount++
				groupByName[group] = "__planned__"
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func ApplyKeycloakOIDCPlan(plan OIDCPlan, host, realm, token string) (*KeycloakApplyResult, error) {
	return ApplyKeycloakOIDCPlanWithOptions(plan, host, realm, token, KeycloakApplyOptions{})
}

func ApplyKeycloakOIDCPlanWithOptions(plan OIDCPlan, host, realm, token string, opts KeycloakApplyOptions) (*KeycloakApplyResult, error) {
	return ApplyKeycloakOIDCPlanWithAuth(plan, host, realm, token, "", "", opts)
}

func ApplyKeycloakOIDCPlanWithAuth(plan OIDCPlan, host, realm, token, username, password string, opts KeycloakApplyOptions) (*KeycloakApplyResult, error) {
	cfg, err := keycloakConfig(host, realm, token, username, password)
	if err != nil {
		return nil, err
	}
	users, err := keycloak.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	groups, err := keycloak.ListGroups(cfg)
	if err != nil {
		return nil, err
	}
	groupByName := map[string]string{}
	for _, group := range groups {
		if rawName := strings.TrimSpace(group.Name); rawName != "" {
			groupByName[rawName] = strings.TrimSpace(group.ID)
			groupByName["/"+rawName] = strings.TrimSpace(group.ID)
		}
		if path := strings.TrimSpace(group.Path); path != "" {
			groupByName[path] = strings.TrimSpace(group.ID)
		}
	}
	result := &KeycloakApplyResult{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Host:      cfg.Host,
		Realm:     cfg.Realm,
		Entries:   make([]KeycloakDiffEntry, 0, len(plan.Users)),
		Notes: []string{
			"Keycloak apply creates missing users, creates missing Tunerr-owned groups, and adds membership from the provider-agnostic OIDC plan.",
		},
	}
	if strings.TrimSpace(opts.BootstrapPassword) != "" {
		result.Notes = append(result.Notes, "A bootstrap password was requested for migrated users.")
	}
	if len(trimNonEmpty(opts.EmailActions)) > 0 {
		result.Notes = append(result.Notes, "Execute-actions-email was requested for migrated users.")
	}
	for _, want := range plan.Users {
		entry := KeycloakDiffEntry{
			SubjectHint:       want.SubjectHint,
			PreferredUsername: want.PreferredUsername,
			Status:            "create_user",
		}
		current := keycloak.FindUserByUsername(users, want.PreferredUsername)
		if current == nil {
			id, err := keycloak.CreateUser(cfg, keycloak.User{
				Username:   want.PreferredUsername,
				Email:      want.Email,
				FirstName:  oidcDisplayFirstName(want.DisplayName),
				LastName:   oidcDisplayLastName(want.DisplayName),
				Attributes: keycloakOIDCAttributes(want),
			})
			if err != nil {
				return nil, fmt.Errorf("create keycloak user %q: %w", want.PreferredUsername, err)
			}
			result.CreateUserCount++
			current = &keycloak.User{ID: id, Username: want.PreferredUsername, Email: want.Email}
		} else {
			entry.Status = "reuse_user"
			fullUser, err := keycloak.GetUser(cfg, current.ID)
			if err != nil {
				return nil, fmt.Errorf("get keycloak user %q: %w", want.PreferredUsername, err)
			}
			if drift := keycloakMetadataDrift(*fullUser, want); len(drift) > 0 {
				update := keycloakDesiredUser(*fullUser, want)
				if err := keycloak.UpdateUser(cfg, current.ID, update); err != nil {
					return nil, fmt.Errorf("update keycloak user %q: %w", want.PreferredUsername, err)
				}
				entry.MetadataUpdate = true
				entry.MetadataDrift = drift
				result.MetadataUpdateCount++
				current = &update
				current.ID = strings.TrimSpace(fullUser.ID)
				current.Enabled = fullUser.Enabled
				current.RequiredActions = append([]string(nil), fullUser.RequiredActions...)
			} else {
				current = fullUser
			}
		}
		entry.ExistingID = strings.TrimSpace(current.ID)
		userGroups, err := keycloak.GetUserGroups(cfg, current.ID)
		if err != nil {
			return nil, fmt.Errorf("get keycloak groups for %q: %w", want.PreferredUsername, err)
		}
		missing := missingKeycloakGroups(want.Groups, userGroups)
		for _, groupName := range missing {
			groupID := strings.TrimSpace(groupByName[groupName])
			if groupID == "" {
				createdID, err := keycloak.CreateGroup(cfg, strings.TrimPrefix(groupName, "/"))
				if err != nil {
					return nil, fmt.Errorf("create keycloak group %q: %w", groupName, err)
				}
				groupID = createdID
				groupByName[groupName] = groupID
				groupByName[strings.TrimPrefix(groupName, "/")] = groupID
				result.CreateGroupCount++
			}
			if err := keycloak.AddUserToGroup(cfg, current.ID, groupID); err != nil {
				return nil, fmt.Errorf("add keycloak membership %q -> %q: %w", want.PreferredUsername, groupName, err)
			}
			result.AddMembershipCount++
		}
		if bootstrap := strings.TrimSpace(opts.BootstrapPassword); bootstrap != "" {
			if err := keycloak.ResetPassword(cfg, current.ID, keycloak.Credential{
				Type:      "password",
				Value:     bootstrap,
				Temporary: opts.PasswordTemporary,
			}); err != nil {
				return nil, fmt.Errorf("bootstrap keycloak password for %q: %w", want.PreferredUsername, err)
			}
		}
		if actions := trimNonEmpty(opts.EmailActions); len(actions) > 0 && strings.TrimSpace(want.Email) != "" {
			if err := keycloak.ExecuteActionsEmail(cfg, current.ID, actions, keycloak.ExecuteActionsEmailOptions{
				ClientID:    strings.TrimSpace(opts.EmailClientID),
				RedirectURI: strings.TrimSpace(opts.EmailRedirectURI),
				LifespanSec: opts.EmailLifespanSec,
			}); err != nil {
				return nil, fmt.Errorf("keycloak execute actions email for %q: %w", want.PreferredUsername, err)
			}
		}
		entry.MissingGroups = missing
		if pending := keycloakActivationPending(*current); pending || entry.Status == "create_user" {
			result.ActivationPendingCount++
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func DiffAuthentikOIDCPlan(plan OIDCPlan, host, token string) (*AuthentikDiffResult, error) {
	cfg, err := authentikConfig(host, token)
	if err != nil {
		return nil, err
	}
	users, err := authentik.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	groups, err := authentik.ListGroups(cfg)
	if err != nil {
		return nil, err
	}
	groupByName := map[string]string{}
	for _, group := range groups {
		if name := strings.TrimSpace(group.Name); name != "" {
			groupByName[name] = strings.TrimSpace(group.ID)
		}
	}
	result := &AuthentikDiffResult{
		ComparedAt: time.Now().UTC().Format(time.RFC3339),
		Host:       cfg.Host,
		Entries:    make([]AuthentikDiffEntry, 0, len(plan.Users)),
		Notes: []string{
			"Authentik diff compares the provider-agnostic OIDC plan against live Authentik users and Tunerr-owned group membership using preferred_username plus stable migration claims.",
		},
	}
	for _, want := range plan.Users {
		entry := AuthentikDiffEntry{
			SubjectHint:       want.SubjectHint,
			PreferredUsername: want.PreferredUsername,
			Status:            "create_user",
		}
		if existing := authentik.FindUserByUsername(users, want.PreferredUsername); existing != nil {
			entry.Status = "reuse_user"
			entry.ExistingID = strings.TrimSpace(existing.ID)
			current, err := authentik.GetUser(cfg, existing.ID)
			if err != nil {
				return nil, fmt.Errorf("get authentik user %q: %w", want.PreferredUsername, err)
			}
			if drift := authentikMetadataDrift(*current, want); len(drift) > 0 {
				entry.MetadataUpdate = true
				entry.MetadataDrift = drift
				result.MetadataUpdateCount++
			}
			missing := missingAuthentikGroups(want.Groups, current.Groups, groups, existing.ID)
			entry.MissingGroups = missing
			if len(missing) > 0 {
				result.AddMembershipCount += len(missing)
			}
		} else {
			result.CreateUserCount++
			if len(want.Groups) > 0 {
				result.AddMembershipCount += len(want.Groups)
				result.ActivationPendingCount++
			}
		}
		for _, group := range want.Groups {
			if _, ok := groupByName[group]; !ok {
				result.CreateGroupCount++
				groupByName[group] = "__planned__"
			}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func ApplyAuthentikOIDCPlan(plan OIDCPlan, host, token string) (*AuthentikApplyResult, error) {
	return ApplyAuthentikOIDCPlanWithOptions(plan, host, token, AuthentikApplyOptions{})
}

func ApplyAuthentikOIDCPlanWithOptions(plan OIDCPlan, host, token string, opts AuthentikApplyOptions) (*AuthentikApplyResult, error) {
	cfg, err := authentikConfig(host, token)
	if err != nil {
		return nil, err
	}
	users, err := authentik.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	groups, err := authentik.ListGroups(cfg)
	if err != nil {
		return nil, err
	}
	groupByName := map[string]string{}
	for _, group := range groups {
		if name := strings.TrimSpace(group.Name); name != "" {
			groupByName[name] = strings.TrimSpace(group.ID)
		}
	}
	result := &AuthentikApplyResult{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Host:      cfg.Host,
		Entries:   make([]AuthentikDiffEntry, 0, len(plan.Users)),
		Notes: []string{
			"Authentik apply creates missing users, creates missing Tunerr-owned groups, and adds membership from the provider-agnostic OIDC plan.",
		},
	}
	if strings.TrimSpace(opts.BootstrapPassword) != "" {
		result.Notes = append(result.Notes, "A bootstrap password was requested for migrated users.")
	}
	if opts.RecoveryEmail {
		result.Notes = append(result.Notes, "Recovery email delivery was requested for migrated users with email addresses.")
	}
	for _, want := range plan.Users {
		entry := AuthentikDiffEntry{
			SubjectHint:       want.SubjectHint,
			PreferredUsername: want.PreferredUsername,
			Status:            "create_user",
		}
		current := authentik.FindUserByUsername(users, want.PreferredUsername)
		if current == nil {
			id, err := authentik.CreateUser(cfg, authentik.User{
				Username:   want.PreferredUsername,
				Name:       strings.TrimSpace(firstNonEmpty(want.DisplayName, want.PreferredUsername)),
				Email:      want.Email,
				Attributes: authentikOIDCAttributes(want),
			})
			if err != nil {
				return nil, fmt.Errorf("create authentik user %q: %w", want.PreferredUsername, err)
			}
			result.CreateUserCount++
			current = &authentik.User{ID: id, Username: want.PreferredUsername, Email: want.Email}
		} else {
			entry.Status = "reuse_user"
		}
		entry.ExistingID = strings.TrimSpace(current.ID)
		currentDetail, err := authentik.GetUser(cfg, current.ID)
		if err != nil {
			return nil, fmt.Errorf("get authentik user %q: %w", want.PreferredUsername, err)
		}
		if drift := authentikMetadataDrift(*currentDetail, want); len(drift) > 0 {
			if err := authentik.UpdateUser(cfg, current.ID, authentikDesiredUserPatch(*currentDetail, want)); err != nil {
				return nil, fmt.Errorf("update authentik user %q: %w", want.PreferredUsername, err)
			}
			entry.MetadataUpdate = true
			entry.MetadataDrift = drift
			result.MetadataUpdateCount++
			currentDetail = &authentik.User{
				ID:         currentDetail.ID,
				Username:   currentDetail.Username,
				Name:       strings.TrimSpace(firstNonEmpty(want.DisplayName, want.PreferredUsername)),
				Email:      strings.TrimSpace(want.Email),
				Attributes: authentikOIDCAttributes(want),
				Groups:     append([]string(nil), currentDetail.Groups...),
			}
		}
		missing := missingAuthentikGroups(want.Groups, currentDetail.Groups, groups, current.ID)
		for _, groupName := range missing {
			groupID := strings.TrimSpace(groupByName[groupName])
			if groupID == "" {
				createdID, err := authentik.CreateGroup(cfg, groupName)
				if err != nil {
					return nil, fmt.Errorf("create authentik group %q: %w", groupName, err)
				}
				groupID = createdID
				groupByName[groupName] = groupID
				result.CreateGroupCount++
			}
			if err := authentik.AddUserToGroup(cfg, groupID, current.ID); err != nil {
				return nil, fmt.Errorf("add authentik membership %q -> %q: %w", want.PreferredUsername, groupName, err)
			}
			result.AddMembershipCount++
		}
		if bootstrap := strings.TrimSpace(opts.BootstrapPassword); bootstrap != "" {
			if err := authentik.SetPassword(cfg, current.ID, bootstrap); err != nil {
				return nil, fmt.Errorf("bootstrap authentik password for %q: %w", want.PreferredUsername, err)
			}
		}
		if opts.RecoveryEmail && strings.TrimSpace(want.Email) != "" {
			if err := authentik.SendRecoveryEmail(cfg, current.ID); err != nil {
				return nil, fmt.Errorf("send authentik recovery email for %q: %w", want.PreferredUsername, err)
			}
		}
		entry.MissingGroups = missing
		if entry.Status == "create_user" {
			result.ActivationPendingCount++
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func DiffPlan(plan Plan, host, token string) (*DiffResult, error) {
	cfg, err := configFromPlan(plan, host, token)
	if err != nil {
		return nil, err
	}
	users, err := emby.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	entries := make([]DiffEntry, 0, len(plan.Users))
	var createCount, reuseCount, conflictCount, policyUpdateCount, activationPendingCount int
	for _, user := range plan.Users {
		entry := DiffEntry{
			PlexID:          user.PlexID,
			DesiredUsername: user.DesiredUsername,
			Status:          "create",
		}
		if existing := emby.FindUserByName(users, user.DesiredUsername); existing != nil {
			entry.Status = "reuse"
			entry.ExistingID = strings.TrimSpace(existing.ID)
			fullUser, err := userWithPolicy(cfg, *existing, user.DesiredPolicy)
			if err != nil {
				return nil, fmt.Errorf("load user policy %q: %w", user.DesiredUsername, err)
			}
			if policy, drift, err := desiredPolicyDrift(fullUser, user.DesiredPolicy); err != nil {
				return nil, fmt.Errorf("diff policy %q: %w", user.DesiredUsername, err)
			} else if len(drift) > 0 {
				_ = policy
				entry.PolicyUpdate = true
				entry.PolicyDrift = drift
			}
			if pending, reason := emby.UserActivationPending(fullUser); pending {
				entry.ActivationPending = true
				entry.ActivationReason = reason
			}
		}
		switch entry.Status {
		case "create":
			createCount++
		case "reuse":
			reuseCount++
		default:
			conflictCount++
		}
		if entry.PolicyUpdate {
			policyUpdateCount++
		}
		if entry.ActivationPending {
			activationPendingCount++
		}
		entries = append(entries, entry)
	}
	return &DiffResult{
		ComparedAt:             time.Now().UTC().Format(time.RFC3339),
		Target:                 cfg.ServerType,
		TargetHost:             cfg.Host,
		CreateCount:            createCount,
		ReuseCount:             reuseCount,
		ConflictCount:          conflictCount,
		PolicyUpdateCount:      policyUpdateCount,
		ActivationPendingCount: activationPendingCount,
		Entries:                entries,
		Notes: []string{
			"Diff is still username-based for account identity, but it now also reports additive destination policy drift plus activation-readiness gaps for existing destination users.",
		},
	}, nil
}

func ApplyPlan(plan Plan, host, token string) (*ApplyResult, error) {
	cfg, err := configFromPlan(plan, host, token)
	if err != nil {
		return nil, err
	}
	users, err := emby.ListUsers(cfg)
	if err != nil {
		return nil, err
	}
	results := make([]ApplyUserResult, 0, len(plan.Users))
	policyUpdateCount := 0
	activationPendingCount := 0
	for _, user := range plan.Users {
		result := ApplyUserResult{
			PlexID:          user.PlexID,
			DesiredUsername: user.DesiredUsername,
		}
		current := emby.FindUserByName(users, user.DesiredUsername)
		if current == nil {
			created, err := emby.CreateUser(cfg, user.DesiredUsername)
			if err != nil {
				return nil, fmt.Errorf("create user %q: %w", user.DesiredUsername, err)
			}
			result.Created = true
			if created != nil {
				result.ID = strings.TrimSpace(created.ID)
				current = created
				users = append(users, *created)
			}
		}
		if current == nil {
			return nil, fmt.Errorf("created or reused user %q is unavailable for follow-up policy sync", user.DesiredUsername)
		}
		result.ID = strings.TrimSpace(current.ID)
		fullUser, err := userWithPolicy(cfg, *current, user.DesiredPolicy)
		if err != nil {
			return nil, fmt.Errorf("load user policy %q: %w", user.DesiredUsername, err)
		}
		updatedPolicy, drift, err := desiredPolicyDrift(fullUser, user.DesiredPolicy)
		if err != nil {
			return nil, fmt.Errorf("apply policy %q: %w", user.DesiredUsername, err)
		}
		if len(drift) > 0 {
			if err := emby.UpdateUserPolicy(cfg, result.ID, updatedPolicy); err != nil {
				return nil, fmt.Errorf("update user policy %q: %w", user.DesiredUsername, err)
			}
			result.PolicyUpdated = true
			result.PolicyDrift = drift
			policyUpdateCount++
		}
		if pending, reason := emby.UserActivationPending(fullUser); pending {
			result.ActivationPending = true
			result.ActivationReason = reason
			activationPendingCount++
		}
		results = append(results, result)
	}
	return &ApplyResult{
		AppliedAt:              time.Now().UTC().Format(time.RFC3339),
		Target:                 cfg.ServerType,
		TargetHost:             cfg.Host,
		PolicyUpdateCount:      policyUpdateCount,
		ActivationPendingCount: activationPendingCount,
		Users:                  results,
		Notes: []string{
			"Applied missing local users and additive destination policy grants that map cleanly from Plex share state.",
			"Activation/invite completion still remains a follow-up step when destination users have no configured password or auto-login path.",
		},
	}, nil
}

func BuildRolloutPlan(bundle Bundle, specs []TargetSpec) (*RolloutPlan, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one target required")
	}
	seen := map[string]bool{}
	plans := make([]Plan, 0, len(specs))
	for _, spec := range specs {
		target := strings.ToLower(strings.TrimSpace(spec.Target))
		if target == "" || seen[target] {
			continue
		}
		plan, err := BuildPlan(bundle, target, spec.Host)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *plan)
		seen[target] = true
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("no valid rollout targets")
	}
	return &RolloutPlan{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		BundleSource: strings.TrimSpace(bundle.Source),
		Plans:        plans,
		Notes: []string{
			"One Plex identity bundle can pre-roll overlapping Emby and Jellyfin local-account creation.",
		},
	}, nil
}

func DiffRolloutPlan(plan RolloutPlan, apply map[string]ApplySpec) (*RolloutDiffResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("rollout plan has no targets")
	}
	results := make([]DiffResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		diff, err := DiffPlan(entry, spec.Host, spec.Token)
		if err != nil {
			return nil, fmt.Errorf("diff %s: %w", entry.Target, err)
		}
		results = append(results, *diff)
	}
	return &RolloutDiffResult{
		ComparedAt: time.Now().UTC().Format(time.RFC3339),
		Results:    results,
		Notes: []string{
			"Rollout diff compares the same Plex-derived identity bundle against multiple destination targets without mutating them.",
		},
	}, nil
}

func ApplyRolloutPlan(plan RolloutPlan, apply map[string]ApplySpec) (*RolloutApplyResult, error) {
	if len(plan.Plans) == 0 {
		return nil, fmt.Errorf("rollout plan has no targets")
	}
	results := make([]ApplyResult, 0, len(plan.Plans))
	for _, entry := range plan.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(entry.Target))]
		res, err := ApplyPlan(entry, spec.Host, spec.Token)
		if err != nil {
			return nil, fmt.Errorf("apply %s: %w", entry.Target, err)
		}
		results = append(results, *res)
	}
	return &RolloutApplyResult{
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Results:   results,
		Notes: []string{
			"Rollout apply intentionally leaves Plex untouched so overlap migration stays possible.",
		},
	}, nil
}

func AuditBundleTargets(bundle Bundle, specs []TargetSpec, apply map[string]ApplySpec) (*AuditResult, error) {
	rollout, err := BuildRolloutPlan(bundle, specs)
	if err != nil {
		return nil, err
	}
	results := make([]TargetAudit, 0, len(rollout.Plans))
	overallStatus := "converged"
	readyToApply := true
	readyTargetCount := 0
	conflictCount := 0
	for _, plan := range rollout.Plans {
		spec := apply[strings.ToLower(strings.TrimSpace(plan.Target))]
		diff, err := DiffPlan(plan, spec.Host, spec.Token)
		if err != nil {
			return nil, fmt.Errorf("audit %s: %w", plan.Target, err)
		}
		audit := summarizeTargetAudit(plan, *diff)
		results = append(results, audit)
		if audit.ReadyToApply {
			readyTargetCount++
		}
		conflictCount += audit.ConflictCount
		switch audit.Status {
		case "blocked_conflicts":
			overallStatus = "blocked_conflicts"
			readyToApply = false
		case "ready_to_apply":
			if overallStatus != "blocked_conflicts" {
				overallStatus = "ready_to_apply"
			}
		}
	}
	return &AuditResult{
		ComparedAt:       time.Now().UTC().Format(time.RFC3339),
		Source:           strings.TrimSpace(bundle.Source),
		Status:           overallStatus,
		ReadyToApply:     readyToApply,
		TargetCount:      len(results),
		ReadyTargetCount: readyTargetCount,
		ConflictCount:    conflictCount,
		Results:          results,
		Notes: []string{
			"Identity audit shows which Plex users still need destination local accounts, which existing accounts still need additive destination policy updates, which ones are not activation-ready yet, and which cases still need manual permission or SSO follow-up.",
		},
	}, nil
}

func AuditOIDCPlanTargets(plan OIDCPlan, specs []OIDCTargetSpec, apply map[string]OIDCApplySpec) (*OIDCAuditResult, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one OIDC target required")
	}
	results := make([]OIDCTargetAudit, 0, len(specs))
	overallStatus := "converged"
	readyToApply := true
	readyTargetCount := 0
	seen := map[string]bool{}
	for _, spec := range specs {
		target := strings.ToLower(strings.TrimSpace(spec.Target))
		if target == "" || seen[target] {
			continue
		}
		resolved := apply[target]
		if strings.TrimSpace(resolved.Host) == "" {
			resolved.Host = strings.TrimSpace(spec.Host)
		}
		if strings.TrimSpace(resolved.Realm) == "" {
			resolved.Realm = strings.TrimSpace(spec.Realm)
		}
		var audit OIDCTargetAudit
		switch target {
		case "keycloak":
			diff, err := DiffKeycloakOIDCPlanWithAuth(plan, resolved.Host, resolved.Realm, resolved.Token, resolved.Username, resolved.Password)
			if err != nil {
				return nil, fmt.Errorf("audit %s: %w", target, err)
			}
			audit = summarizeKeycloakOIDCAudit(*diff)
		case "authentik":
			diff, err := DiffAuthentikOIDCPlan(plan, resolved.Host, resolved.Token)
			if err != nil {
				return nil, fmt.Errorf("audit %s: %w", target, err)
			}
			audit = summarizeAuthentikOIDCAudit(*diff)
		default:
			return nil, fmt.Errorf("unsupported OIDC target %q", spec.Target)
		}
		results = append(results, audit)
		if audit.ReadyToApply {
			readyTargetCount++
		}
		if audit.Status == "ready_to_apply" && overallStatus != "blocked_conflicts" {
			overallStatus = "ready_to_apply"
		}
		if !audit.ReadyToApply {
			readyToApply = false
		}
		seen[target] = true
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no valid OIDC targets")
	}
	return &OIDCAuditResult{
		ComparedAt:       time.Now().UTC().Format(time.RFC3339),
		Issuer:           strings.TrimSpace(plan.Issuer),
		ClientID:         strings.TrimSpace(plan.ClientID),
		Status:           overallStatus,
		ReadyToApply:     readyToApply,
		TargetCount:      len(results),
		ReadyTargetCount: readyTargetCount,
		Results:          results,
		Notes: []string{
			"OIDC audit compares the neutral identity-provider plan against live IdP targets so account bootstrap, group creation, and membership drift can be reviewed before apply.",
			"Current OIDC audit is provisioning-focused; it does not claim full downstream SSO-policy parity.",
		},
	}, nil
}

func FormatAuditSummary(result AuditResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Identity migration audit\n")
	fmt.Fprintf(&b, "overall_status: %s\n", strings.TrimSpace(result.Status))
	fmt.Fprintf(&b, "ready_to_apply: %t\n", result.ReadyToApply)
	fmt.Fprintf(&b, "targets: %d\n", result.TargetCount)
	fmt.Fprintf(&b, "ready_targets: %d\n", result.ReadyTargetCount)
	fmt.Fprintf(&b, "conflicts: %d\n", result.ConflictCount)
	for _, target := range result.Results {
		fmt.Fprintf(&b, "\n[%s] %s\n", strings.TrimSpace(target.Target), strings.TrimSpace(target.TargetHost))
		fmt.Fprintf(&b, "status: %s\n", strings.TrimSpace(target.Status))
		if reason := strings.TrimSpace(target.StatusReason); reason != "" {
			fmt.Fprintf(&b, "reason: %s\n", reason)
		}
		fmt.Fprintf(&b, "create_count: %d\n", target.CreateCount)
		fmt.Fprintf(&b, "reuse_count: %d\n", target.ReuseCount)
		fmt.Fprintf(&b, "policy_update_count: %d\n", target.PolicyUpdateCount)
		fmt.Fprintf(&b, "activation_pending_count: %d\n", target.ActivationPendingCount)
		fmt.Fprintf(&b, "manual_follow_up_count: %d\n", target.ManualFollowUpCount)
		if len(target.MissingUsers) > 0 {
			fmt.Fprintf(&b, "missing_users: %s\n", strings.Join(target.MissingUsers, ", "))
		}
		if len(target.PolicyUpdateUsers) > 0 {
			fmt.Fprintf(&b, "policy_update_users: %s\n", strings.Join(target.PolicyUpdateUsers, ", "))
		}
		if len(target.ActivationPendingUsers) > 0 {
			fmt.Fprintf(&b, "activation_pending_users: %s\n", strings.Join(target.ActivationPendingUsers, ", "))
		}
		if len(target.ManualFollowUpUsers) > 0 {
			names := make([]string, 0, len(target.ManualFollowUpUsers))
			for _, user := range target.ManualFollowUpUsers {
				names = append(names, user.DesiredUsername)
			}
			fmt.Fprintf(&b, "manual_follow_up_users: %s\n", strings.Join(names, ", "))
		}
	}
	return b.String()
}

func FormatOIDCAuditSummary(result OIDCAuditResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "OIDC migration audit\n")
	fmt.Fprintf(&b, "overall_status: %s\n", strings.TrimSpace(result.Status))
	fmt.Fprintf(&b, "ready_to_apply: %t\n", result.ReadyToApply)
	if issuer := strings.TrimSpace(result.Issuer); issuer != "" {
		fmt.Fprintf(&b, "issuer: %s\n", issuer)
	}
	if clientID := strings.TrimSpace(result.ClientID); clientID != "" {
		fmt.Fprintf(&b, "client_id: %s\n", clientID)
	}
	fmt.Fprintf(&b, "targets: %d\n", result.TargetCount)
	fmt.Fprintf(&b, "ready_targets: %d\n", result.ReadyTargetCount)
	for _, target := range result.Results {
		fmt.Fprintf(&b, "\n[%s] %s\n", strings.TrimSpace(target.Target), strings.TrimSpace(target.TargetHost))
		fmt.Fprintf(&b, "status: %s\n", strings.TrimSpace(target.Status))
		if reason := strings.TrimSpace(target.StatusReason); reason != "" {
			fmt.Fprintf(&b, "reason: %s\n", reason)
		}
		fmt.Fprintf(&b, "create_user_count: %d\n", target.CreateUserCount)
		fmt.Fprintf(&b, "create_group_count: %d\n", target.CreateGroupCount)
		fmt.Fprintf(&b, "add_membership_count: %d\n", target.AddMembershipCount)
		fmt.Fprintf(&b, "metadata_update_count: %d\n", target.MetadataUpdateCount)
		fmt.Fprintf(&b, "activation_pending_count: %d\n", target.ActivationPendingCount)
		if len(target.MissingUsers) > 0 {
			fmt.Fprintf(&b, "missing_users: %s\n", strings.Join(target.MissingUsers, ", "))
		}
		if len(target.MissingGroups) > 0 {
			fmt.Fprintf(&b, "missing_groups: %s\n", strings.Join(target.MissingGroups, ", "))
		}
		if len(target.MembershipUsers) > 0 {
			fmt.Fprintf(&b, "membership_users: %s\n", strings.Join(target.MembershipUsers, ", "))
		}
		if len(target.MetadataUsers) > 0 {
			fmt.Fprintf(&b, "metadata_users: %s\n", strings.Join(target.MetadataUsers, ", "))
		}
	}
	return b.String()
}

func Load(path string) (*Bundle, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	var bundle Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func configFromPlan(plan Plan, host, token string) (emby.Config, error) {
	target := strings.ToLower(strings.TrimSpace(plan.Target))
	if target != "emby" && target != "jellyfin" {
		return emby.Config{}, fmt.Errorf("target must be emby or jellyfin")
	}
	cfg := emby.Config{
		Host:       strings.TrimSpace(firstNonEmpty(host, plan.TargetHost)),
		Token:      strings.TrimSpace(token),
		ServerType: target,
	}
	if cfg.Host == "" {
		return emby.Config{}, fmt.Errorf("target host required")
	}
	if cfg.Token == "" {
		return emby.Config{}, fmt.Errorf("target token required")
	}
	return cfg, nil
}

func summarizeTargetAudit(plan Plan, diff DiffResult) TargetAudit {
	target := TargetAudit{
		Target:                 strings.TrimSpace(diff.Target),
		TargetHost:             strings.TrimSpace(diff.TargetHost),
		Diff:                   diff,
		CreateCount:            diff.CreateCount,
		ReuseCount:             diff.ReuseCount,
		ConflictCount:          diff.ConflictCount,
		PolicyUpdateCount:      diff.PolicyUpdateCount,
		ActivationPendingCount: diff.ActivationPendingCount,
	}
	missing := make([]string, 0, diff.CreateCount)
	policyUsers := make([]string, 0, diff.PolicyUpdateCount)
	activationUsers := make([]string, 0, diff.ActivationPendingCount)
	for _, entry := range diff.Entries {
		if entry.Status == "create" {
			missing = append(missing, entry.DesiredUsername)
		}
		if entry.PolicyUpdate {
			policyUsers = append(policyUsers, entry.DesiredUsername)
		}
		if entry.ActivationPending {
			activationUsers = append(activationUsers, entry.DesiredUsername)
		}
	}
	target.MissingUsers = missing
	target.PolicyUpdateUsers = policyUsers
	target.ActivationPendingUsers = activationUsers
	manual := make([]AuditUser, 0)
	for _, user := range plan.Users {
		if user.ServerShared {
			target.SharedUserCount++
		}
		if user.Managed {
			target.ManagedUserCount++
		}
		if user.AllowTuners {
			target.TunerEntitledCount++
		}
		if needsManualFollowUp(user) {
			manual = append(manual, AuditUser{
				PlexID:          user.PlexID,
				DesiredUsername: user.DesiredUsername,
				Managed:         user.Managed,
				ServerShared:    user.ServerShared,
				AllowTuners:     user.AllowTuners,
				AllowSync:       user.AllowSync,
				AllLibraries:    user.AllLibraries,
			})
		}
	}
	target.ManualFollowUpUsers = manual
	target.ManualFollowUpCount = len(manual)
	switch {
	case target.ConflictCount > 0:
		target.Status = "blocked_conflicts"
		target.StatusReason = "destination account conflicts exist"
	case target.CreateCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some Plex users still need destination accounts"
	case target.PolicyUpdateCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some destination accounts still need access policy updates"
	case target.ActivationPendingCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "all destination accounts exist, but some users are still not activation-ready"
	default:
		target.Status = "converged"
		if target.ManualFollowUpCount > 0 {
			target.StatusReason = "all destination accounts exist, but some users still need permission or SSO follow-up"
		} else {
			target.StatusReason = "all destination accounts already exist"
		}
	}
	target.ReadyToApply = target.ConflictCount == 0
	return target
}

func summarizeKeycloakOIDCAudit(diff KeycloakDiffResult) OIDCTargetAudit {
	target := OIDCTargetAudit{
		Target:                 "keycloak",
		TargetHost:             strings.TrimSpace(diff.Host),
		CreateUserCount:        diff.CreateUserCount,
		CreateGroupCount:       diff.CreateGroupCount,
		AddMembershipCount:     diff.AddMembershipCount,
		MetadataUpdateCount:    diff.MetadataUpdateCount,
		ActivationPendingCount: diff.ActivationPendingCount,
	}
	missingUsers := make([]string, 0, diff.CreateUserCount)
	missingGroups := map[string]bool{}
	membershipUsers := make([]string, 0)
	metadataUsers := make([]string, 0)
	for _, entry := range diff.Entries {
		if entry.Status == "create_user" {
			missingUsers = append(missingUsers, entry.PreferredUsername)
		}
		if len(entry.MissingGroups) > 0 {
			membershipUsers = append(membershipUsers, entry.PreferredUsername)
			for _, group := range entry.MissingGroups {
				group = strings.TrimSpace(group)
				if group != "" {
					missingGroups[group] = true
				}
			}
		}
		if entry.MetadataUpdate {
			metadataUsers = append(metadataUsers, entry.PreferredUsername)
		}
	}
	target.MissingUsers = missingUsers
	target.MembershipUsers = membershipUsers
	target.MetadataUsers = metadataUsers
	target.MissingGroups = sortedKeys(missingGroups)
	switch {
	case target.CreateUserCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some IdP users still need to be created"
	case target.CreateGroupCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some Tunerr migration groups are still missing"
	case target.AddMembershipCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some IdP users still need Tunerr group membership"
	case target.MetadataUpdateCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some existing IdP users still need Tunerr metadata updates"
	case target.ActivationPendingCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "IdP users exist, but some still show onboarding-required signals"
	default:
		target.Status = "converged"
		target.StatusReason = "all IdP users and Tunerr groups already exist"
	}
	target.ReadyToApply = true
	return target
}

func summarizeAuthentikOIDCAudit(diff AuthentikDiffResult) OIDCTargetAudit {
	target := OIDCTargetAudit{
		Target:                 "authentik",
		TargetHost:             strings.TrimSpace(diff.Host),
		CreateUserCount:        diff.CreateUserCount,
		CreateGroupCount:       diff.CreateGroupCount,
		AddMembershipCount:     diff.AddMembershipCount,
		MetadataUpdateCount:    diff.MetadataUpdateCount,
		ActivationPendingCount: diff.ActivationPendingCount,
	}
	missingUsers := make([]string, 0, diff.CreateUserCount)
	missingGroups := map[string]bool{}
	membershipUsers := make([]string, 0)
	metadataUsers := make([]string, 0)
	for _, entry := range diff.Entries {
		if entry.Status == "create_user" {
			missingUsers = append(missingUsers, entry.PreferredUsername)
		}
		if len(entry.MissingGroups) > 0 {
			membershipUsers = append(membershipUsers, entry.PreferredUsername)
			for _, group := range entry.MissingGroups {
				group = strings.TrimSpace(group)
				if group != "" {
					missingGroups[group] = true
				}
			}
		}
		if entry.MetadataUpdate {
			metadataUsers = append(metadataUsers, entry.PreferredUsername)
		}
	}
	target.MissingUsers = missingUsers
	target.MembershipUsers = membershipUsers
	target.MetadataUsers = metadataUsers
	target.MissingGroups = sortedKeys(missingGroups)
	switch {
	case target.CreateUserCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some IdP users still need to be created"
	case target.CreateGroupCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some Tunerr migration groups are still missing"
	case target.AddMembershipCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some IdP users still need Tunerr group membership"
	case target.MetadataUpdateCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "some existing IdP users still need Tunerr metadata updates"
	case target.ActivationPendingCount > 0:
		target.Status = "ready_to_apply"
		target.StatusReason = "newly created IdP users still need onboarding completion"
	default:
		target.Status = "converged"
		target.StatusReason = "all IdP users and Tunerr groups already exist"
	}
	target.ReadyToApply = true
	return target
}

func desiredPolicyForUser(user BundleUser) DesiredPolicy {
	var policy DesiredPolicy
	if user.AllowTuners {
		policy.EnableLiveTvAccess = boolPtr(true)
	}
	if user.ServerShared {
		policy.EnableRemoteAccess = boolPtr(true)
	}
	if user.AllowSync {
		policy.EnableContentDownloading = boolPtr(true)
		policy.EnableSyncTranscoding = boolPtr(true)
	}
	if user.AllLibraries {
		policy.EnableAllFolders = boolPtr(true)
	}
	return policy
}

func (p DesiredPolicy) hasAnyValue() bool {
	return p.EnableLiveTvAccess != nil ||
		p.EnableRemoteAccess != nil ||
		p.EnableContentDownloading != nil ||
		p.EnableSyncTranscoding != nil ||
		p.EnableAllFolders != nil
}

func (p DesiredPolicy) toEmbyDesiredPolicy() emby.DesiredUserPolicy {
	return emby.DesiredUserPolicy{
		EnableLiveTvAccess:       p.EnableLiveTvAccess,
		EnableRemoteAccess:       p.EnableRemoteAccess,
		EnableContentDownloading: p.EnableContentDownloading,
		EnableSyncTranscoding:    p.EnableSyncTranscoding,
		EnableAllFolders:         p.EnableAllFolders,
	}
}

func desiredPolicyDrift(user emby.UserInfo, desired DesiredPolicy) (map[string]any, []string, error) {
	if !desired.hasAnyValue() {
		return nil, nil, nil
	}
	merged, drift, err := emby.MergeDesiredUserPolicy(user.Policy, desired.toEmbyDesiredPolicy())
	if err != nil {
		return nil, nil, err
	}
	return merged, drift, nil
}

func needsManualFollowUp(user PlanUser) bool {
	if user.Managed {
		return true
	}
	if user.ServerShared && !user.AllLibraries {
		return true
	}
	return false
}

func boolPtr(v bool) *bool { return &v }

func userWithPolicy(cfg emby.Config, user emby.UserInfo, desired DesiredPolicy) (emby.UserInfo, error) {
	if !desired.hasAnyValue() || len(user.Policy) > 0 {
		return user, nil
	}
	if strings.TrimSpace(user.ID) == "" {
		return user, fmt.Errorf("user id missing")
	}
	full, err := emby.GetUser(cfg, user.ID)
	if err != nil {
		return user, err
	}
	if full == nil {
		return user, fmt.Errorf("user lookup returned nil")
	}
	return *full, nil
}

func oidcSubjectHint(user BundleUser) string {
	if uuid := strings.TrimSpace(user.PlexUUID); uuid != "" {
		return "plex:" + uuid
	}
	if user.PlexID > 0 {
		return fmt.Sprintf("plex:id:%d", user.PlexID)
	}
	name := strings.TrimSpace(firstNonEmpty(user.DesiredUsername, user.Username, user.Title, user.Email))
	if name == "" {
		return "plex:unknown"
	}
	return "plex:name:" + name
}

func oidcGroupsForUser(user BundleUser) []string {
	groups := []string{"tunerr:migrated"}
	if user.Home {
		groups = append(groups, "tunerr:plex-home")
	}
	if user.Managed {
		groups = append(groups, "tunerr:plex-managed")
	}
	if user.Restricted {
		groups = append(groups, "tunerr:plex-restricted")
	}
	if user.ServerShared {
		groups = append(groups, "tunerr:plex-shared")
	}
	if user.AllowTuners {
		groups = append(groups, "tunerr:live-tv")
	}
	if user.AllowSync {
		groups = append(groups, "tunerr:sync")
	}
	if user.AllLibraries {
		groups = append(groups, "tunerr:all-libraries")
	}
	slices.Sort(groups)
	return slices.Compact(groups)
}

func keycloakConfig(host, realm, token, username, password string) (keycloak.Config, error) {
	return keycloak.ResolveConfig(keycloak.Config{
		Host:     strings.TrimSpace(host),
		Realm:    strings.TrimSpace(realm),
		Token:    strings.TrimSpace(token),
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
	})
}

func authentikConfig(host, token string) (authentik.Config, error) {
	cfg := authentik.Config{
		Host:  strings.TrimSpace(host),
		Token: strings.TrimSpace(token),
	}
	if cfg.Host == "" {
		return authentik.Config{}, fmt.Errorf("authentik host required")
	}
	if cfg.Token == "" {
		return authentik.Config{}, fmt.Errorf("authentik token required")
	}
	return cfg, nil
}

func missingKeycloakGroups(want []string, current []keycloak.Group) []string {
	if len(want) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, group := range current {
		if name := strings.TrimSpace(group.Name); name != "" {
			have[name] = true
			have["/"+name] = true
		}
		if path := strings.TrimSpace(group.Path); path != "" {
			have[path] = true
		}
	}
	out := make([]string, 0, len(want))
	for _, group := range want {
		group = strings.TrimSpace(group)
		if group == "" || have[group] {
			continue
		}
		out = append(out, group)
	}
	return out
}

func missingAuthentikGroups(want []string, current []string, groups []authentik.Group, userID string) []string {
	if len(want) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, group := range current {
		group = strings.TrimSpace(group)
		if group != "" {
			have[group] = true
		}
	}
	for _, group := range groups {
		if !containsString(group.Users, userID) {
			continue
		}
		if name := strings.TrimSpace(group.Name); name != "" {
			have[name] = true
		}
		if id := strings.TrimSpace(group.ID); id != "" {
			have[id] = true
		}
	}
	out := make([]string, 0, len(want))
	for _, group := range want {
		group = strings.TrimSpace(group)
		if group == "" || have[group] {
			continue
		}
		out = append(out, group)
	}
	return out
}

func keycloakActivationPending(user keycloak.User) bool {
	return !user.Enabled || len(trimNonEmpty(user.RequiredActions)) > 0
}

func keycloakOIDCAttributes(user OIDCPlanUser) map[string][]string {
	attrs := map[string][]string{
		"tunerr_subject_hint":       {strings.TrimSpace(user.SubjectHint)},
		"tunerr_preferred_username": {strings.TrimSpace(user.PreferredUsername)},
		"tunerr_source":             {"plex_identity_migration"},
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		attrs["tunerr_email_hint"] = []string{email}
	}
	if uuid := strings.TrimSpace(user.PlexUUID); uuid != "" {
		attrs["tunerr_plex_uuid"] = []string{uuid}
	}
	if user.PlexID > 0 {
		attrs["tunerr_plex_id"] = []string{fmt.Sprintf("%d", user.PlexID)}
	}
	if len(user.Groups) > 0 {
		attrs["tunerr_groups"] = trimNonEmpty(user.Groups)
	}
	return attrs
}

func keycloakDesiredUser(current keycloak.User, want OIDCPlanUser) keycloak.User {
	out := current
	out.Username = strings.TrimSpace(want.PreferredUsername)
	out.Email = strings.TrimSpace(want.Email)
	out.FirstName = oidcDisplayFirstName(want.DisplayName)
	out.LastName = oidcDisplayLastName(want.DisplayName)
	out.Attributes = keycloakOIDCAttributes(want)
	return out
}

func keycloakMetadataDrift(current keycloak.User, want OIDCPlanUser) []string {
	drift := make([]string, 0, 4)
	if strings.TrimSpace(current.Email) != strings.TrimSpace(want.Email) {
		drift = append(drift, "email")
	}
	if strings.TrimSpace(current.FirstName) != oidcDisplayFirstName(want.DisplayName) || strings.TrimSpace(current.LastName) != oidcDisplayLastName(want.DisplayName) {
		drift = append(drift, "display_name")
	}
	if !equalStringSliceMap(current.Attributes, keycloakOIDCAttributes(want)) {
		drift = append(drift, "tunerr_attributes")
	}
	return drift
}

func authentikOIDCAttributes(user OIDCPlanUser) map[string]any {
	attrs := map[string]any{
		"tunerr_subject_hint":       strings.TrimSpace(user.SubjectHint),
		"tunerr_preferred_username": strings.TrimSpace(user.PreferredUsername),
		"tunerr_source":             "plex_identity_migration",
	}
	if email := strings.TrimSpace(user.Email); email != "" {
		attrs["tunerr_email_hint"] = email
	}
	if uuid := strings.TrimSpace(user.PlexUUID); uuid != "" {
		attrs["tunerr_plex_uuid"] = uuid
	}
	if user.PlexID > 0 {
		attrs["tunerr_plex_id"] = user.PlexID
	}
	if len(user.Groups) > 0 {
		attrs["tunerr_groups"] = trimNonEmpty(user.Groups)
	}
	return attrs
}

func authentikDesiredUserPatch(current authentik.User, want OIDCPlanUser) map[string]any {
	patch := map[string]any{}
	desiredName := strings.TrimSpace(firstNonEmpty(want.DisplayName, want.PreferredUsername))
	if strings.TrimSpace(current.Name) != desiredName {
		patch["name"] = desiredName
	}
	if strings.TrimSpace(current.Email) != strings.TrimSpace(want.Email) {
		patch["email"] = strings.TrimSpace(want.Email)
	}
	if !equalAnyMap(current.Attributes, authentikOIDCAttributes(want)) {
		patch["attributes"] = authentikOIDCAttributes(want)
	}
	return patch
}

func authentikMetadataDrift(current authentik.User, want OIDCPlanUser) []string {
	drift := make([]string, 0, 3)
	desiredName := strings.TrimSpace(firstNonEmpty(want.DisplayName, want.PreferredUsername))
	if strings.TrimSpace(current.Name) != desiredName {
		drift = append(drift, "display_name")
	}
	if strings.TrimSpace(current.Email) != strings.TrimSpace(want.Email) {
		drift = append(drift, "email")
	}
	if !equalAnyMap(current.Attributes, authentikOIDCAttributes(want)) {
		drift = append(drift, "tunerr_attributes")
	}
	return drift
}

func oidcDisplayFirstName(display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return ""
	}
	parts := strings.Fields(display)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func oidcDisplayLastName(display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return ""
	}
	parts := strings.Fields(display)
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], " ")
}

func trimNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	slices.Sort(out)
	return out
}

func equalStringSliceMap(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, want := range b {
		got, ok := a[key]
		if !ok || !slices.Equal(trimNonEmpty(got), trimNonEmpty(want)) {
			return false
		}
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}

func equalAnyMap(a, b map[string]any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func desiredUsername(id int, username, title, email string) string {
	if trimmed := strings.TrimSpace(username); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(email); trimmed != "" {
		if before, _, ok := strings.Cut(trimmed, "@"); ok && strings.TrimSpace(before) != "" {
			return strings.TrimSpace(before)
		}
		return trimmed
	}
	if id > 0 {
		return fmt.Sprintf("plex-user-%d", id)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
