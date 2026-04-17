package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/livetvbundle"
	"github.com/snapetech/iptvtunerr/internal/migrationident"
)

func (s *Server) migrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "migration_audit",
		Steps: []string{
			"Build or refresh the neutral migration bundle from Plex before trusting any overlap-status result.",
			"Set IPTV_TUNERR_MIGRATION_BUNDLE_FILE plus the target Emby/Jellyfin host and token envs on the running process.",
			"Use the reported ready/converged state to decide whether the non-Plex side is only pre-rolled, blocked by conflicts, or materially caught up.",
			"Inspect count-lagging and title-lagging libraries before cutting users over, especially when reused destination libraries already exist.",
		},
		Actions: []string{
			"/api/debug/runtime.json",
			"/deck/migration-audit.json",
		},
	}

	bundlePath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_MIGRATION_BUNDLE_FILE"))
	if bundlePath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_MIGRATION_BUNDLE_FILE to enable migration audit workflow reporting",
		}))
		return
	}

	specs := []livetvbundle.TargetSpec{}
	apply := map[string]livetvbundle.ApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_HOST")); host != "" {
		specs = append(specs, livetvbundle.TargetSpec{Target: "emby", Host: host})
		apply["emby"] = livetvbundle.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")); host != "" {
		specs = append(specs, livetvbundle.TargetSpec{Target: "jellyfin", Host: host})
		apply["jellyfin"] = livetvbundle.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       "configure IPTV_TUNERR_EMBY_HOST and/or IPTV_TUNERR_JELLYFIN_HOST to audit migration targets",
		}))
		return
	}

	bundle, err := livetvbundle.Load(bundlePath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	audit, err := livetvbundle.AuditBundleTargets(*bundle, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  true,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                  result.Target,
			"target_host":             result.TargetHost,
			"status":                  result.Status,
			"ready_to_apply":          result.ReadyToApply,
			"status_reason":           result.StatusReason,
			"indexed_channel_count":   result.LiveTV.IndexedChannelCount,
			"missing_libraries":       result.MissingLibraries,
			"lagging_libraries":       result.LaggingLibraries,
			"title_lagging_libraries": result.TitleLaggingLibraries,
			"empty_libraries":         result.EmptyLibraries,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"bundle_file":        bundlePath,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"conflict_count":     audit.ConflictCount,
		"targets":            targets,
		"report":             livetvbundle.FormatMigrationAuditSummary(*audit),
	}))
}

func (s *Server) identityMigrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "identity_migration_audit",
		Steps: []string{
			"Build or refresh the Plex-user identity bundle before trusting any account-cutover status.",
			"Set IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE plus the target Emby/Jellyfin host and token envs on the running process.",
			"Use the audit to separate missing destination accounts from users that already exist but still need permission, invite, or SSO follow-up.",
			"Do not treat ready-to-apply as password or OIDC parity; this lane currently covers local-account existence and follow-up hints only.",
		},
		Actions: []string{
			"/api/debug/runtime.json",
			"/deck/identity-migration-audit.json",
		},
	}

	bundlePath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE"))
	if bundlePath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_IDENTITY_MIGRATION_BUNDLE_FILE to enable identity migration workflow reporting",
		}))
		return
	}

	specs := []migrationident.TargetSpec{}
	apply := map[string]migrationident.ApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_HOST")); host != "" {
		specs = append(specs, migrationident.TargetSpec{Target: "emby", Host: host})
		apply["emby"] = migrationident.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_EMBY_TOKEN"))}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_HOST")); host != "" {
		specs = append(specs, migrationident.TargetSpec{Target: "jellyfin", Host: host})
		apply["jellyfin"] = migrationident.ApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_JELLYFIN_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       "configure IPTV_TUNERR_EMBY_HOST and/or IPTV_TUNERR_JELLYFIN_HOST to audit identity migration targets",
		}))
		return
	}

	bundle, err := migrationident.Load(bundlePath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  false,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	audit, err := migrationident.AuditBundleTargets(*bundle, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured":  true,
			"bundle_file": bundlePath,
			"error":       err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                 result.Target,
			"target_host":            result.TargetHost,
			"status":                 result.Status,
			"ready_to_apply":         result.ReadyToApply,
			"status_reason":          result.StatusReason,
			"create_count":           result.CreateCount,
			"reuse_count":            result.ReuseCount,
			"missing_users":          result.MissingUsers,
			"manual_follow_up_count": result.ManualFollowUpCount,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"bundle_file":        bundlePath,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"conflict_count":     audit.ConflictCount,
		"targets":            targets,
		"report":             migrationident.FormatAuditSummary(*audit),
	}))
}

func (s *Server) oidcMigrationAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	report := deckWorkflowReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Name:        "oidc_migration_audit",
		Steps: []string{
			"Build or refresh the provider-agnostic OIDC plan before trusting any IdP migration state.",
			"Set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE plus the Keycloak/Authentik host and token envs on the running process.",
			"Use the audit to separate missing IdP users, missing Tunerr migration groups, and missing group membership before apply.",
			"Do not treat converged as full SSO-policy parity; this lane currently covers account, group, and onboarding bootstrap state only.",
		},
		Actions: []string{
			"/deck/oidc-migration-audit.json",
		},
	}

	planPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE"))
	if planPath == "" {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"error":      "set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE to enable OIDC migration workflow reporting",
		}))
		return
	}

	planData, err := os.ReadFile(planPath)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}
	var plan migrationident.OIDCPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}

	specs := []migrationident.OIDCTargetSpec{}
	apply := map[string]migrationident.OIDCApplySpec{}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")); host != "" {
		realm := strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM"))
		specs = append(specs, migrationident.OIDCTargetSpec{Target: "keycloak", Host: host, Realm: realm})
		apply["keycloak"] = migrationident.OIDCApplySpec{
			Host:     host,
			Realm:    realm,
			Token:    strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")),
			Username: strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")),
			Password: strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")),
		}
	}
	if host := strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")); host != "" {
		specs = append(specs, migrationident.OIDCTargetSpec{Target: "authentik", Host: host})
		apply["authentik"] = migrationident.OIDCApplySpec{Host: host, Token: strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN"))}
	}
	if len(specs) == 0 {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": false,
			"plan_file":  planPath,
			"error":      "configure IPTV_TUNERR_KEYCLOAK_HOST and/or IPTV_TUNERR_AUTHENTIK_HOST to audit OIDC migration targets",
		}))
		return
	}

	audit, err := migrationident.AuditOIDCPlanTargets(plan, specs, apply)
	if err != nil {
		writeJSONPayload(w, report.withSummary(map[string]interface{}{
			"configured": true,
			"plan_file":  planPath,
			"error":      err.Error(),
		}))
		return
	}
	targets := make([]map[string]interface{}, 0, len(audit.Results))
	for _, result := range audit.Results {
		targets = append(targets, map[string]interface{}{
			"target":                   result.Target,
			"target_host":              result.TargetHost,
			"status":                   result.Status,
			"ready_to_apply":           result.ReadyToApply,
			"status_reason":            result.StatusReason,
			"create_user_count":        result.CreateUserCount,
			"create_group_count":       result.CreateGroupCount,
			"add_membership_count":     result.AddMembershipCount,
			"activation_pending_count": result.ActivationPendingCount,
			"missing_users":            result.MissingUsers,
			"missing_groups":           result.MissingGroups,
			"membership_users":         result.MembershipUsers,
		})
	}
	lastApply := s.lastActivityByTitle("oidc_migration_apply")
	lastApplySummary := map[string]interface{}{}
	if lastApply != nil {
		lastApplySummary["at"] = lastApply.At
		lastApplySummary["message"] = lastApply.Message
		lastApplySummary["detail"] = lastApply.Detail
	}
	recentApplies := make([]map[string]interface{}, 0)
	for _, entry := range s.lastActivitiesByTitle("oidc_migration_apply", 4) {
		recentApplies = append(recentApplies, map[string]interface{}{
			"at":      entry.At,
			"message": entry.Message,
			"detail":  entry.Detail,
		})
	}
	writeJSONPayload(w, report.withSummary(map[string]interface{}{
		"configured":         true,
		"plan_file":          planPath,
		"issuer":             audit.Issuer,
		"client_id":          audit.ClientID,
		"overall_status":     audit.Status,
		"ready_to_apply":     audit.ReadyToApply,
		"target_count":       audit.TargetCount,
		"ready_target_count": audit.ReadyTargetCount,
		"targets":            targets,
		"last_apply":         lastApplySummary,
		"recent_applies":     recentApplies,
		"report":             migrationident.FormatOIDCAuditSummary(*audit),
	}))
}

func (s *Server) oidcMigrationApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		writeMethodNotAllowedJSON(w, http.MethodPost)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	planPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE"))
	if planPath == "" {
		writeJSONError(w, http.StatusBadRequest, "set IPTV_TUNERR_IDENTITY_OIDC_PLAN_FILE first")
		return
	}
	var req deckOIDCApplyRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read apply body")
		return
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid apply json")
			return
		}
	}
	planData, err := os.ReadFile(planPath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var plan migrationident.OIDCPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	targets := map[string]bool{}
	results := map[string]any{}
	for _, target := range req.Targets {
		if trimmed := strings.ToLower(strings.TrimSpace(target)); trimmed != "" {
			targets[trimmed] = true
		}
	}
	if len(targets) == 0 {
		if strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")) != "" {
			targets["keycloak"] = true
		}
		if strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")) != "" {
			targets["authentik"] = true
		}
	}
	if len(targets) == 0 {
		writeJSONError(w, http.StatusBadRequest, "configure IPTV_TUNERR_KEYCLOAK_HOST and/or IPTV_TUNERR_AUTHENTIK_HOST first")
		return
	}
	if req.Keycloak.EmailLifespanSec < 0 {
		s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, "keycloak email_lifespan_sec must be non-negative", "validate")
		writeJSONError(w, http.StatusBadRequest, "keycloak email_lifespan_sec must be non-negative")
		return
	}

	if targets["keycloak"] {
		keycloakOpts := migrationident.KeycloakApplyOptions{
			BootstrapPassword: strings.TrimSpace(req.Keycloak.BootstrapPassword),
			PasswordTemporary: true,
			EmailActions:      append([]string(nil), req.Keycloak.EmailActions...),
			EmailClientID:     strings.TrimSpace(req.Keycloak.EmailClientID),
			EmailRedirectURI:  strings.TrimSpace(req.Keycloak.EmailRedirectURI),
			EmailLifespanSec:  req.Keycloak.EmailLifespanSec,
		}
		if req.Keycloak.PasswordTemporary != nil {
			keycloakOpts.PasswordTemporary = *req.Keycloak.PasswordTemporary
		}
		res, err := migrationident.ApplyKeycloakOIDCPlanWithAuth(plan,
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_HOST")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_REALM")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_TOKEN")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_USER")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_KEYCLOAK_PASSWORD")),
			keycloakOpts,
		)
		if err != nil {
			s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, err.Error(), "apply_keycloak")
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		results["keycloak"] = res
	}
	if targets["authentik"] {
		authentikOpts := migrationident.AuthentikApplyOptions{
			BootstrapPassword: strings.TrimSpace(req.Authentik.BootstrapPassword),
			RecoveryEmail:     req.Authentik.RecoveryEmail,
		}
		res, err := migrationident.ApplyAuthentikOIDCPlanWithOptions(plan,
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_HOST")),
			strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTHENTIK_TOKEN")),
			authentikOpts,
		)
		if err != nil {
			s.recordOIDCMigrationApplyActivity(false, "OIDC migration apply failed.", targets, req, results, err.Error(), "apply_authentik")
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		results["authentik"] = res
	}
	s.recordOIDCMigrationApplyActivity(true, "OIDC migration apply executed from the deck.", targets, req, results, "", "")
	writeJSONPayload(w, map[string]any{
		"ok":      true,
		"action":  "oidc_migration_apply",
		"message": "OIDC migration apply completed.",
		"targets": sortedStringMapKeys(targets),
		"results": results,
	})
}

func (r deckWorkflowReport) withSummary(summary map[string]interface{}) deckWorkflowReport {
	r.Summary = summary
	return r
}

func writeJSONPayload(w http.ResponseWriter, value interface{}) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "encode json payload")
		return
	}
	_, _ = w.Write(body)
}

func sortedStringMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key, ok := range values {
		if ok && strings.TrimSpace(key) != "" {
			out = append(out, strings.TrimSpace(key))
		}
	}
	slices.Sort(out)
	return out
}

func (s *Server) recordOIDCMigrationApplyActivity(ok bool, message string, targets map[string]bool, req deckOIDCApplyRequest, results map[string]any, applyErr string, phase string) {
	targetNames := sortedStringMapKeys(targets)
	detail := map[string]interface{}{
		"ok":      ok,
		"targets": targetNames,
		"keycloak": map[string]interface{}{
			"bootstrap_password": strings.TrimSpace(req.Keycloak.BootstrapPassword) != "",
			"password_temporary": req.Keycloak.PasswordTemporary == nil || *req.Keycloak.PasswordTemporary,
			"email_actions":      req.Keycloak.EmailActions,
			"email_client_id":    strings.TrimSpace(req.Keycloak.EmailClientID),
			"email_redirect_uri": strings.TrimSpace(req.Keycloak.EmailRedirectURI),
			"email_lifespan_sec": req.Keycloak.EmailLifespanSec,
		},
		"authentik": map[string]interface{}{
			"bootstrap_password": strings.TrimSpace(req.Authentik.BootstrapPassword) != "",
			"recovery_email":     req.Authentik.RecoveryEmail,
		},
		"result_targets":  oidcApplyResultSummaryMap(results),
		"target_statuses": oidcApplyTargetStatusMap(targetNames, results, applyErr, phase),
	}
	if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
		detail["error"] = trimmed
	}
	if trimmed := strings.TrimSpace(phase); trimmed != "" {
		detail["phase"] = trimmed
	}
	s.recordActivity("oidc_migration", "oidc_migration_apply", message, detail)
}

func oidcApplyResultSummaryMap(results map[string]any) map[string]interface{} {
	summary := map[string]interface{}{}
	for target, raw := range results {
		switch result := raw.(type) {
		case *migrationident.KeycloakApplyResult:
			if result == nil {
				continue
			}
			summary[target] = map[string]interface{}{
				"create_user_count":        result.CreateUserCount,
				"create_group_count":       result.CreateGroupCount,
				"add_membership_count":     result.AddMembershipCount,
				"metadata_update_count":    result.MetadataUpdateCount,
				"activation_pending_count": result.ActivationPendingCount,
			}
		case *migrationident.AuthentikApplyResult:
			if result == nil {
				continue
			}
			summary[target] = map[string]interface{}{
				"create_user_count":        result.CreateUserCount,
				"create_group_count":       result.CreateGroupCount,
				"add_membership_count":     result.AddMembershipCount,
				"metadata_update_count":    result.MetadataUpdateCount,
				"activation_pending_count": result.ActivationPendingCount,
			}
		}
	}
	return summary
}

func oidcApplyTargetStatusMap(targets []string, results map[string]any, applyErr string, phase string) map[string]interface{} {
	statuses := map[string]interface{}{}
	failedTarget := ""
	switch strings.TrimSpace(phase) {
	case "apply_keycloak":
		failedTarget = "keycloak"
	case "apply_authentik":
		failedTarget = "authentik"
	}
	resultSummaries := oidcApplyResultSummaryMap(results)
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if result, ok := resultSummaries[target].(map[string]interface{}); ok {
			row := map[string]interface{}{
				"status": "applied",
			}
			for key, value := range result {
				row[key] = value
			}
			statuses[target] = row
			continue
		}
		row := map[string]interface{}{}
		switch {
		case failedTarget != "" && target == failedTarget:
			row["status"] = "failed"
			if trimmed := strings.TrimSpace(phase); trimmed != "" {
				row["phase"] = trimmed
			}
			if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
				row["error"] = trimmed
			}
		case strings.TrimSpace(phase) == "validate":
			row["status"] = "validation_failed"
			if trimmed := strings.TrimSpace(applyErr); trimmed != "" {
				row["error"] = trimmed
			}
		default:
			row["status"] = "not_reached"
			if failedTarget != "" {
				row["blocked_by"] = failedTarget
			}
		}
		statuses[target] = row
	}
	return statuses
}

func (s *Server) lastActivityByTitle(title string) *DeckActivityEntry {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	for i := len(s.activityEntries) - 1; i >= 0; i-- {
		entry := s.activityEntries[i]
		if strings.TrimSpace(entry.Title) != title {
			continue
		}
		copyEntry := copyDeckActivityEntry(entry)
		return &copyEntry
	}
	return nil
}

func (s *Server) lastActivitiesByTitle(title string, limit int) []DeckActivityEntry {
	title = strings.TrimSpace(title)
	if title == "" || limit <= 0 {
		return nil
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	out := make([]DeckActivityEntry, 0, limit)
	for i := len(s.activityEntries) - 1; i >= 0 && len(out) < limit; i-- {
		entry := s.activityEntries[i]
		if strings.TrimSpace(entry.Title) != title {
			continue
		}
		out = append(out, copyDeckActivityEntry(entry))
	}
	return out
}

func copyDeckActivityEntry(entry DeckActivityEntry) DeckActivityEntry {
	copyEntry := entry
	if entry.Detail != nil {
		copyEntry.Detail = map[string]interface{}{}
		for key, value := range entry.Detail {
			copyEntry.Detail[key] = value
		}
	}
	return copyEntry
}
