package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	stepDriftAgent     bool
	stepDriftNudge     bool
	stepDriftPeek      bool
	stepDriftRig       string
	stepDriftThreshold int
	stepDriftWatch     bool
)

// stepsOrder defines the canonical molecule step names in execution order.
var stepsOrder = []string{
	"Load context",
	"Set up working branch",
	"Verify tests pass",
	"Implement",
	"Self-review",
	"Run tests",
	"Clean up",
	"Prepare work",
	"Submit work",
}

const stepLabels = "①load ②branch ③preflight ④implement ⑤review ⑥test ⑦cleanup ⑧prepare ⑨submit"

const nudgeMsg = "You have been working for several minutes with no molecule steps closed. " +
	"Close each step IMMEDIATELY when you finish it: `bd close <step-id>`. " +
	"Run `bd ready` to see your next step. Not closing steps signals you are " +
	"not following the formula."

var patrolStepDriftCmd = &cobra.Command{
	Use:   "step-drift [interval]",
	Short: "Detect polecats with unclosed molecule steps",
	Long: `Detect and nudge polecats with unclosed molecule steps.

Reads polecat step status from their isolated Dolt branches (not main)
to get true closure state. Detects "step drift" — when a polecat has been
working for a threshold duration without closing any steps.

Examples:
  gt patrol step-drift                  # Human-readable display
  gt patrol step-drift --peek           # Include recent polecat output
  gt patrol step-drift --rig gastown    # Only check polecats in one rig
  gt patrol step-drift --watch          # Live dashboard, refresh every 30s
  gt patrol step-drift --watch 10       # Custom refresh interval
  gt patrol step-drift --agent          # JSON report (for deacon/scripts)
  gt patrol step-drift --agent --nudge  # JSON report + nudge drifting polecats
  gt patrol step-drift --threshold 8    # Custom drift threshold (default: 5 min)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPatrolStepDrift,
}

// StepDriftResult represents the drift status of a single polecat.
type StepDriftResult struct {
	Rig      string  `json:"rig"`
	Name     string  `json:"name"`
	Bead     string  `json:"bead"`
	Title    string  `json:"title"`
	State    string  `json:"state"`
	AgeMin   float64 `json:"age_min"`
	Closed   int     `json:"closed"`
	Total    int     `json:"total"`
	Drifting bool    `json:"drifting"`
	Nudged   bool    `json:"nudged"`
	Branch   string  `json:"branch"`
	Error    string  `json:"error,omitempty"`
}

func init() {
	patrolStepDriftCmd.Flags().BoolVar(&stepDriftAgent, "agent", false, "JSON output for deacon/scripts")
	patrolStepDriftCmd.Flags().BoolVar(&stepDriftNudge, "nudge", false, "Nudge drifting polecats")
	patrolStepDriftCmd.Flags().BoolVar(&stepDriftPeek, "peek", false, "Include recent polecat output in human-readable mode")
	patrolStepDriftCmd.Flags().StringVar(&stepDriftRig, "rig", "", "Only check polecats in this rig")
	patrolStepDriftCmd.Flags().IntVar(&stepDriftThreshold, "threshold", 5, "Drift threshold in minutes")
	patrolStepDriftCmd.Flags().BoolVarP(&stepDriftWatch, "watch", "w", false, "Live dashboard mode")
}

func runPatrolStepDrift(cmd *cobra.Command, args []string) error {
	interval := 30
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			interval = v
		}
	}

	if stepDriftWatch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		for {
			// Clear screen
			fmt.Print("\033[2J\033[H")
			fmt.Printf("patrol-step-drift  (%s)\n", time.Now().Format("15:04:05"))
			fmt.Println(strings.Repeat("=", 80))

			results := checkStepDrift(stepDriftThreshold)
			if stepDriftNudge {
				nudgeDrifting(results)
			}
			renderStepDriftPretty(results)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Duration(interval) * time.Second):
			}
		}
	}

	results := checkStepDrift(stepDriftThreshold)
	if stepDriftNudge {
		nudgeDrifting(results)
	}

	if stepDriftAgent {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	fmt.Println("patrol-step-drift")
	fmt.Println(strings.Repeat("=", 80))
	renderStepDriftPretty(results)
	return nil
}

// checkStepDrift checks all polecats for step drift.
func checkStepDrift(thresholdMinutes int) []StepDriftResult {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil
	}

	polecats := listAllPolecats()
	if stepDriftRig != "" {
		var filtered []polecatInfo
		for _, p := range polecats {
			if p.rig == stepDriftRig {
				filtered = append(filtered, p)
			}
		}
		polecats = filtered
	}

	var results []StepDriftResult
	for _, p := range polecats {
		branch := findDoltBranch(townRoot, p.rig, p.name)
		wispID := findWispID(p.bead)
		statuses := readStepStatus(wispID, branch)
		closed := countClosedSteps(statuses)
		age := sessionAgeMinutes(p.rig, p.name)

		var errMsg string
		if branch == "" && p.bead != "" {
			errMsg = "could not find Dolt branch"
		} else if wispID == "" && p.bead != "" {
			errMsg = "could not find attached molecule"
		}

		results = append(results, StepDriftResult{
			Rig:      p.rig,
			Name:     p.name,
			Bead:     p.bead,
			Title:    fetchBeadTitle(p.bead),
			State:    p.state,
			AgeMin:   roundTo1(age),
			Closed:   closed,
			Total:    len(stepsOrder),
			Drifting: age >= float64(thresholdMinutes) && closed == 0,
			Nudged:   false,
			Branch:   branch,
			Error:    errMsg,
		})
	}
	return results
}

// nudgeDrifting sends nudge messages to drifting polecats.
func nudgeDrifting(results []StepDriftResult) {
	for i := range results {
		if results[i].Drifting {
			target := fmt.Sprintf("%s/%s", results[i].Rig, results[i].Name)
			cmd := exec.Command("gt", "nudge", target, nudgeMsg)
			_ = cmd.Run()
			results[i].Nudged = true
		}
	}
}

// polecatInfo holds basic info about a polecat from gt polecat list.
type polecatInfo struct {
	rig   string
	name  string
	state string
	bead  string
}

// listAllPolecats returns all working polecats across all rigs.
func listAllPolecats() []polecatInfo {
	rigs := listRigs()
	var all []polecatInfo
	for _, rig := range rigs {
		all = append(all, listPolecatsForRig(rig)...)
	}
	return all
}

// listRigs returns the names of all rigs.
func listRigs() []string {
	out, err := exec.Command("gt", "rig", "list", "--json").Output()
	if err != nil {
		return nil
	}
	var rigs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &rigs); err != nil {
		return nil
	}
	names := make([]string, len(rigs))
	for i, r := range rigs {
		names[i] = r.Name
	}
	return names
}

// listPolecatsForRig returns polecats for a single rig.
func listPolecatsForRig(rig string) []polecatInfo {
	out, err := exec.Command("gt", "polecat", "list", rig, "--json").Output()
	if err != nil {
		return nil
	}
	var data []struct {
		Rig   string `json:"rig"`
		Name  string `json:"name"`
		State string `json:"state"`
		Issue string `json:"issue"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil
	}
	result := make([]polecatInfo, len(data))
	for i, p := range data {
		rigName := p.Rig
		if rigName == "" {
			rigName = rig
		}
		result[i] = polecatInfo{
			rig:   rigName,
			name:  p.Name,
			state: p.State,
			bead:  p.Issue,
		}
	}
	return result
}

// findDoltBranch finds the most recent Dolt branch for a polecat via SQL server.
// Uses the running Dolt server rather than CLI against the data directory.
func findDoltBranch(townRoot, rig, name string) string {
	doltDataDir := filepath.Join(townRoot, ".dolt-data")
	prefix := fmt.Sprintf("polecat-%s-%%", strings.ToLower(name))
	query := fmt.Sprintf("USE %s; SELECT name FROM dolt_branches WHERE name LIKE '%s' ORDER BY name DESC LIMIT 1", rig, prefix)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", query, "-r", "csv")
	cmd.Dir = doltDataDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse CSV output: header line then data line
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "name" && strings.HasPrefix(line, "polecat-") {
			return line
		}
	}
	return ""
}

// fetchBeadTitle extracts the title from a bead's show output.
func fetchBeadTitle(beadID string) string {
	if beadID == "" {
		return "?"
	}
	out, err := exec.Command("bd", "show", beadID).Output()
	if err != nil {
		return "?"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, beadID) {
			re := regexp.MustCompile(`·\s*(.+?)\s*\[`)
			if m := re.FindStringSubmatch(line); len(m) > 1 {
				title := m[1]
				if len(title) > 80 {
					title = title[:80]
				}
				return title
			}
		}
	}
	return "?"
}

// findWispID finds the attached molecule/wisp ID for a bead.
func findWispID(beadID string) string {
	if beadID == "" {
		return ""
	}
	out, err := exec.Command("bd", "show", beadID).Output()
	if err != nil {
		return ""
	}
	lines := string(out)

	// Try attached_molecule field first
	reAttached := regexp.MustCompile(`attached_molecule:\s*(\S+)`)
	if m := reAttached.FindStringSubmatch(lines); len(m) > 1 {
		return m[1]
	}

	// Fallback: look for wisp- with mol-polecat-work
	reWisp := regexp.MustCompile(`(\S+-wisp-\S+)`)
	for _, line := range strings.Split(lines, "\n") {
		if strings.Contains(line, "wisp-") && strings.Contains(line, "mol-polecat-work") {
			if m := reWisp.FindStringSubmatch(line); len(m) > 1 {
				return strings.TrimRight(m[1], ":")
			}
		}
	}
	return ""
}

// readStepStatus reads step closure status from a wisp, optionally on a Dolt branch.
func readStepStatus(wispID, doltBranch string) map[string]bool {
	if wispID == "" {
		return nil
	}

	cmd := exec.Command("bd", "show", wispID)
	if doltBranch != "" {
		cmd.Env = append(os.Environ(), "BD_BRANCH="+doltBranch)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	statuses := make(map[string]bool)
	reStep := regexp.MustCompile(`:\s*(.+?)\s*●`)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "↳") {
			continue
		}
		closed := strings.Contains(line, "✓")
		if m := reStep.FindStringSubmatch(line); len(m) > 1 {
			statuses[strings.TrimSpace(m[1])] = closed
		}
	}
	return statuses
}

// countClosedSteps counts how many canonical steps are closed.
func countClosedSteps(statuses map[string]bool) int {
	count := 0
	for _, step := range stepsOrder {
		if matchStep(step, statuses) {
			count++
		}
	}
	return count
}

// matchStep checks if a canonical step name matches any key in statuses and is closed.
func matchStep(stepName string, statuses map[string]bool) bool {
	lower := strings.ToLower(stepName)
	for key, closed := range statuses {
		if strings.Contains(strings.ToLower(key), lower) {
			return closed
		}
	}
	return false
}

// sessionAgeMinutes returns how long a polecat's tmux session has been alive.
func sessionAgeMinutes(rig, name string) float64 {
	sessionName := fmt.Sprintf("gt-%s-%s", rig, name)
	out, err := exec.Command("tmux", "display-message", "-t", sessionName,
		"-p", "#{session_created}").Output()
	if err != nil {
		return 0
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return time.Since(time.Unix(ts, 0)).Minutes()
}

// peekPolecat returns recent output from a polecat session.
func peekPolecat(rig, name string, lines int) string {
	target := fmt.Sprintf("%s/%s", rig, name)
	out, err := exec.Command("gt", "peek", target, "-n", strconv.Itoa(lines)).Output()
	if err != nil {
		return ""
	}
	var filtered []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "⚠ gt binary") || strings.HasPrefix(strings.TrimSpace(line), "→ Run") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// renderStepDriftPretty renders human-readable output.
func renderStepDriftPretty(results []StepDriftResult) {
	if len(results) == 0 {
		fmt.Println("  No active polecats.")
		return
	}

	for _, p := range results {
		var progressStr string
		for i := 0; i < p.Total; i++ {
			if i < p.Closed {
				progressStr += "●"
			} else {
				progressStr += "○"
			}
		}

		ageStr := ""
		if p.AgeMin > 0 {
			ageStr = fmt.Sprintf("%dm", int(p.AgeMin))
		}
		stateStr := ""
		if p.State != "working" {
			stateStr = fmt.Sprintf("(%s)", p.State)
		}
		title := p.Title
		if len(title) > 55 {
			title = title[:55]
		}

		fmt.Printf("  ▶ %-10s %-12s %s  %s %s %s\n",
			p.Name, p.Bead, progressStr, title, stateStr, ageStr)

		if p.Error != "" {
			fmt.Printf("    %s\n", style.Warning.Render("⚠ "+p.Error))
		}

		if stepDriftPeek {
			peek := peekPolecat(p.Rig, p.Name, 20)
			if peek != "" {
				lines := strings.Split(peek, "\n")
				var tail []string
				for _, l := range lines {
					if strings.TrimSpace(l) != "" {
						tail = append(tail, l)
					}
				}
				if len(tail) > 20 {
					tail = tail[len(tail)-20:]
				}
				for _, line := range tail {
					if len(line) > 100 {
						line = line[:100]
					}
					fmt.Printf("    │ %s\n", line)
				}
			}
		}

		if p.Drifting {
			fmt.Printf("    %s\n", style.Warning.Render(fmt.Sprintf("⚡ Step drift detected (%dm, 0 steps closed)", int(p.AgeMin))))
		}
		if p.Nudged {
			fmt.Printf("    %s\n", style.Warning.Render("⚡ Nudged"))
		}
		fmt.Println()
	}

	fmt.Printf("  Steps: %s\n", stepLabels)
	fmt.Println("  ● = done  ○ = pending  ⚡ = drifting")
}

// roundTo1 rounds a float to 1 decimal place.
func roundTo1(f float64) float64 {
	return float64(int(f*10)) / 10
}
