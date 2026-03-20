package tuner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type GhostHunterRecoveryResult struct {
	Mode     string `json:"mode"`
	Path     string `json:"path"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

func (cfg GhostHunterConfig) GhostHunterReady() bool {
	return strings.TrimSpace(cfg.PMSURL) != "" && strings.TrimSpace(cfg.Token) != ""
}

func ghostHunterRecoveryHelperPath() string {
	if path := strings.TrimSpace(os.Getenv("IPTV_TUNERR_GHOST_HUNTER_RECOVERY_HELPER")); path != "" {
		return path
	}
	return "./scripts/plex-hidden-grab-recover.sh"
}

func RunGhostHunterRecoveryHelper(ctx context.Context, mode string) (GhostHunterRecoveryResult, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "off" || mode == "none" {
		return GhostHunterRecoveryResult{}, nil
	}
	args := []string{}
	switch mode {
	case "dry-run":
		args = append(args, "--dry-run")
	case "restart":
		args = append(args, "--restart")
	default:
		return GhostHunterRecoveryResult{}, fmt.Errorf("unknown recover-hidden mode %q", mode)
	}
	path := ghostHunterRecoveryHelperPath()
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	result := GhostHunterRecoveryResult{
		Mode:   mode,
		Path:   path,
		Output: strings.TrimSpace(string(out)),
	}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, err
}
