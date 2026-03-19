package tuner

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type CatchupRecorderLaneReport struct {
	Lane             string `json:"lane"`
	ActiveCount      int    `json:"active_count"`
	CompletedCount   int    `json:"completed_count"`
	FailedCount      int    `json:"failed_count"`
	InterruptedCount int    `json:"interrupted_count"`
	PublishedCount   int    `json:"published_count"`
}

type CatchupRecorderReport struct {
	GeneratedAt      string                      `json:"generated_at"`
	StateFile        string                      `json:"state_file"`
	RootDir          string                      `json:"root_dir,omitempty"`
	UpdatedAt        string                      `json:"updated_at,omitempty"`
	Statistics       CatchupRecorderStatistics   `json:"statistics"`
	PublishedCount   int                         `json:"published_count"`
	InterruptedCount int                         `json:"interrupted_count"`
	Lanes            []CatchupRecorderLaneReport `json:"lanes"`
	Active           []CatchupRecorderItem       `json:"active"`
	Completed        []CatchupRecorderItem       `json:"completed"`
	Failed           []CatchupRecorderItem       `json:"failed"`
}

func LoadCatchupRecorderState(path string) (CatchupRecorderState, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return CatchupRecorderState{}, fmt.Errorf("state file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CatchupRecorderState{}, err
	}
	var state CatchupRecorderState
	if err := json.Unmarshal(data, &state); err != nil {
		return CatchupRecorderState{}, err
	}
	return state, nil
}

func BuildCatchupRecorderReport(state CatchupRecorderState, stateFile string, limit int) CatchupRecorderReport {
	if limit <= 0 {
		limit = 10
	}
	report := CatchupRecorderReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		StateFile:   strings.TrimSpace(stateFile),
		RootDir:     strings.TrimSpace(state.RootDir),
		UpdatedAt:   strings.TrimSpace(state.UpdatedAt),
		Statistics:  state.Statistics,
		Active:      truncateCatchupRecorderItems(state.Active, limit),
		Completed:   truncateCatchupRecorderItems(state.Completed, limit),
		Failed:      truncateCatchupRecorderItems(state.Failed, limit),
	}
	byLane := map[string]*CatchupRecorderLaneReport{}
	visit := func(items []CatchupRecorderItem, bucket string) {
		for _, item := range items {
			lane := firstNonEmptyString(item.Lane, "general")
			row := byLane[lane]
			if row == nil {
				row = &CatchupRecorderLaneReport{Lane: lane}
				byLane[lane] = row
			}
			switch bucket {
			case "active":
				row.ActiveCount++
			case "completed":
				row.CompletedCount++
				if strings.TrimSpace(item.PublishedPath) != "" {
					row.PublishedCount++
					report.PublishedCount++
				}
			case "failed":
				row.FailedCount++
				if strings.ToLower(strings.TrimSpace(item.Status)) == "interrupted" {
					row.InterruptedCount++
					report.InterruptedCount++
				}
			}
		}
	}
	visit(state.Active, "active")
	visit(state.Completed, "completed")
	visit(state.Failed, "failed")
	for _, row := range byLane {
		report.Lanes = append(report.Lanes, *row)
	}
	sort.SliceStable(report.Lanes, func(i, j int) bool { return report.Lanes[i].Lane < report.Lanes[j].Lane })
	return report
}

func LoadCatchupRecorderReport(path string, limit int) (CatchupRecorderReport, error) {
	state, err := LoadCatchupRecorderState(path)
	if err != nil {
		return CatchupRecorderReport{}, err
	}
	return BuildCatchupRecorderReport(state, path, limit), nil
}

func truncateCatchupRecorderItems(items []CatchupRecorderItem, limit int) []CatchupRecorderItem {
	if len(items) <= limit {
		return append([]CatchupRecorderItem(nil), items...)
	}
	return append([]CatchupRecorderItem(nil), items[:limit]...)
}
