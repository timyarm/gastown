package cmd

import (
	"testing"
)

func TestStepDriftCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range patrolCmd.Commands() {
		if cmd.Use == "step-drift [interval]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("step-drift subcommand not registered under patrol")
	}
}

func TestStepDriftCmd_HasFlags(t *testing.T) {
	flags := []string{"agent", "nudge", "peek", "rig", "threshold", "watch"}
	for _, name := range flags {
		if patrolStepDriftCmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag: --%s", name)
		}
	}
	if patrolStepDriftCmd.Flags().ShorthandLookup("w") == nil {
		t.Error("missing shorthand -w for --watch")
	}
}

func TestStepDriftCmd_ThresholdDefault(t *testing.T) {
	f := patrolStepDriftCmd.Flags().Lookup("threshold")
	if f == nil {
		t.Fatal("threshold flag not found")
	}
	if f.DefValue != "5" {
		t.Errorf("threshold default = %q, want %q", f.DefValue, "5")
	}
}

func TestMatchStep(t *testing.T) {
	statuses := map[string]bool{
		"Load context and start":       true,
		"Set up working branch":        true,
		"Verify tests pass (precheck)": false,
		"Implement the feature":        false,
	}

	tests := []struct {
		name string
		want bool
	}{
		{"Load context", true},
		{"Set up working branch", true},
		{"Verify tests pass", false},
		{"Implement", false},
		{"Self-review", false}, // not present at all
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchStep(tt.name, statuses)
			if got != tt.want {
				t.Errorf("matchStep(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestCountClosedSteps(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string]bool
		want     int
	}{
		{
			name: "all closed",
			statuses: map[string]bool{
				"Load context":          true,
				"Set up working branch": true,
				"Verify tests pass":     true,
				"Implement":             true,
				"Self-review":           true,
				"Run tests":             true,
				"Clean up":              true,
				"Prepare work":          true,
				"Submit work":           true,
			},
			want: 9,
		},
		{
			name: "none closed",
			statuses: map[string]bool{
				"Load context":          false,
				"Set up working branch": false,
			},
			want: 0,
		},
		{
			name:     "nil map",
			statuses: nil,
			want:     0,
		},
		{
			name: "partial with fuzzy names",
			statuses: map[string]bool{
				"Load context and verify assignment": true,
				"Set up working branch":              true,
				"Verify tests pass on base branch":   true,
				"Implement the solution":             false,
				"Self-review changes":                false,
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countClosedSteps(tt.statuses); got != tt.want {
				t.Errorf("countClosedSteps() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRoundTo1(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{12.34, 12.3},
		{0.0, 0.0},
		{5.99, 5.9},
		{100.05, 100.0},
	}
	for _, tt := range tests {
		got := roundTo1(tt.input)
		if got != tt.want {
			t.Errorf("roundTo1(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStepDriftResult_ErrorField(t *testing.T) {
	r := StepDriftResult{
		Rig:   "gastown",
		Name:  "alpha",
		Error: "could not find Dolt branch",
	}
	if r.Error == "" {
		t.Error("Error field should not be empty")
	}

	r2 := StepDriftResult{
		Rig:  "gastown",
		Name: "beta",
	}
	if r2.Error != "" {
		t.Errorf("Error field should be empty, got %q", r2.Error)
	}
}

func TestStepsOrder(t *testing.T) {
	if len(stepsOrder) != 9 {
		t.Errorf("stepsOrder has %d entries, want 9", len(stepsOrder))
	}
	if stepsOrder[0] != "Load context" {
		t.Errorf("first step = %q, want %q", stepsOrder[0], "Load context")
	}
	if stepsOrder[8] != "Submit work" {
		t.Errorf("last step = %q, want %q", stepsOrder[8], "Submit work")
	}
}

func TestReadStepStatus_EmptyWisp(t *testing.T) {
	// Should return nil for empty wisp ID without making any external calls
	result := readStepStatus("", "some-branch")
	if result != nil {
		t.Errorf("readStepStatus with empty wispID should return nil, got %v", result)
	}
}

func TestMatchStep_CaseInsensitive(t *testing.T) {
	statuses := map[string]bool{
		"LOAD CONTEXT AND VERIFY": true,
		"run tests (quality)":     false,
	}

	if !matchStep("Load context", statuses) {
		t.Error("matchStep should be case-insensitive")
	}
	if matchStep("Run tests", statuses) {
		t.Error("matchStep should return false for unclosed steps")
	}
}
