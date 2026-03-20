package tuner

import (
	"encoding/json"
	"os"
	"strings"
)

type autopilotHostPolicy struct {
	PreferredHosts []string `json:"preferred_hosts,omitempty"`
	BlockedHosts   []string `json:"blocked_hosts,omitempty"`
}

type autopilotHostPolicyFile struct {
	GlobalPreferredHosts []string `json:"global_preferred_hosts,omitempty"`
	GlobalBlockedHosts   []string `json:"global_blocked_hosts,omitempty"`
	PreferredHosts       []string `json:"preferred_hosts,omitempty"`
	BlockedHosts         []string `json:"blocked_hosts,omitempty"`
}

func parseAutopilotHostPolicy() autopilotHostPolicy {
	policy := autopilotHostPolicy{
		PreferredHosts: normalizeAutopilotHostList(strings.Split(strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTOPILOT_GLOBAL_PREFERRED_HOSTS")), ",")),
	}
	path := strings.TrimSpace(os.Getenv("IPTV_TUNERR_AUTOPILOT_HOST_POLICY_FILE"))
	if path == "" {
		return policy
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return policy
	}
	var filePolicy autopilotHostPolicyFile
	if err := json.Unmarshal(data, &filePolicy); err != nil {
		return policy
	}
	policy.PreferredHosts = normalizeAutopilotHostList(append(policy.PreferredHosts, append(filePolicy.GlobalPreferredHosts, filePolicy.PreferredHosts...)...))
	policy.BlockedHosts = normalizeAutopilotHostList(append(filePolicy.GlobalBlockedHosts, filePolicy.BlockedHosts...))
	return policy
}

func normalizeAutopilotHostList(hosts []string) []string {
	if len(hosts) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hosts))
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterAutopilotBlockedHosts(urls []string, blockedHosts []string) []string {
	if len(urls) < 2 || len(blockedHosts) == 0 {
		return urls
	}
	blocked := make(map[string]struct{}, len(blockedHosts))
	for _, host := range blockedHosts {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" {
			blocked[host] = struct{}{}
		}
	}
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		host := strings.ToLower(strings.TrimSpace(autopilotURLHost(u)))
		if _, ok := blocked[host]; ok {
			continue
		}
		out = append(out, u)
	}
	if len(out) == 0 {
		return urls
	}
	return out
}
