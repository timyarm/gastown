package crew

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

// Common errors
var (
	ErrCrewExists   = errors.New("crew worker already exists")
	ErrCrewNotFound = errors.New("crew worker not found")
	ErrHasChanges   = errors.New("crew worker has uncommitted changes")
)

// Manager handles crew worker lifecycle.
type Manager struct {
	rig *rig.Rig
	git *git.Git
}

// NewManager creates a new crew manager.
func NewManager(r *rig.Rig, g *git.Git) *Manager {
	return &Manager{
		rig: r,
		git: g,
	}
}

// crewDir returns the directory for a crew worker.
func (m *Manager) crewDir(name string) string {
	return filepath.Join(m.rig.Path, "crew", name)
}

// stateFile returns the state file path for a crew worker.
func (m *Manager) stateFile(name string) string {
	return filepath.Join(m.crewDir(name), "state.json")
}

// mailDir returns the mail directory path for a crew worker.
func (m *Manager) mailDir(name string) string {
	return filepath.Join(m.crewDir(name), "mail")
}

// exists checks if a crew worker exists.
func (m *Manager) exists(name string) bool {
	_, err := os.Stat(m.crewDir(name))
	return err == nil
}

// Add creates a new crew worker with a clone of the rig.
func (m *Manager) Add(name string, createBranch bool) (*CrewWorker, error) {
	if m.exists(name) {
		return nil, ErrCrewExists
	}

	crewPath := m.crewDir(name)

	// Create crew directory if needed
	crewBaseDir := filepath.Join(m.rig.Path, "crew")
	if err := os.MkdirAll(crewBaseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating crew dir: %w", err)
	}

	// Clone the rig repo
	if err := m.git.Clone(m.rig.GitURL, crewPath); err != nil {
		return nil, fmt.Errorf("cloning rig: %w", err)
	}

	crewGit := git.NewGit(crewPath)
	branchName := "main"

	// Optionally create a working branch
	if createBranch {
		branchName = fmt.Sprintf("crew/%s", name)
		if err := crewGit.CreateBranch(branchName); err != nil {
			os.RemoveAll(crewPath)
			return nil, fmt.Errorf("creating branch: %w", err)
		}
		if err := crewGit.Checkout(branchName); err != nil {
			os.RemoveAll(crewPath)
			return nil, fmt.Errorf("checking out branch: %w", err)
		}
	}

	// Create mail directory for mail delivery
	mailPath := m.mailDir(name)
	if err := os.MkdirAll(mailPath, 0755); err != nil {
		os.RemoveAll(crewPath)
		return nil, fmt.Errorf("creating mail dir: %w", err)
	}

	// Create CLAUDE.md with crew worker prompting
	if err := m.createClaudeMD(name, crewPath); err != nil {
		os.RemoveAll(crewPath)
		return nil, fmt.Errorf("creating CLAUDE.md: %w", err)
	}

	// Create crew worker state
	now := time.Now()
	crew := &CrewWorker{
		Name:      name,
		Rig:       m.rig.Name,
		ClonePath: crewPath,
		Branch:    branchName,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save state
	if err := m.saveState(crew); err != nil {
		os.RemoveAll(crewPath)
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return crew, nil
}

// createClaudeMD creates the CLAUDE.md file for crew worker prompting.
func (m *Manager) createClaudeMD(name, crewPath string) error {
	content := fmt.Sprintf(`# Claude: Crew Worker - %s

You are a **crew worker** in the %s rig. Crew workers are user-managed persistent workspaces.

## Key Differences from Polecats

- **User-managed**: You are NOT managed by the Witness daemon
- **Persistent**: Your workspace is not automatically cleaned up
- **Optional issue assignment**: You can work without a beads issue
- **Mail enabled**: You can send and receive mail

## Commands

Check mail:
`+"```bash"+`
town mail inbox --as %s/%s
`+"```"+`

Send mail:
`+"```bash"+`
town mail send <recipient> -s "Subject" -m "Message" --as %s/%s
`+"```"+`

## Session Cycling (Handoff)

When your context fills up, use mail-to-self for handoff:

1. Compose a handoff note with current state
2. Send to yourself: `+"```"+`town mail send %s/%s -s "Handoff" -m "..."--as %s/%s`+"```"+`
3. Exit cleanly
4. New session reads handoff from inbox

## Beads

If using beads for task tracking:
`+"```bash"+`
bd ready           # Find available work
bd show <id>       # Review issue details
bd update <id> --status=in_progress  # Claim it
bd close <id>      # Mark complete
`+"```"+`
`, name, m.rig.Name,
		m.rig.Name, name,
		m.rig.Name, name,
		m.rig.Name, name, m.rig.Name, name)

	claudePath := filepath.Join(crewPath, "CLAUDE.md")
	return os.WriteFile(claudePath, []byte(content), 0644)
}

// Remove deletes a crew worker.
func (m *Manager) Remove(name string, force bool) error {
	if !m.exists(name) {
		return ErrCrewNotFound
	}

	crewPath := m.crewDir(name)

	if !force {
		crewGit := git.NewGit(crewPath)
		hasChanges, err := crewGit.HasUncommittedChanges()
		if err == nil && hasChanges {
			return ErrHasChanges
		}
	}

	// Remove directory
	if err := os.RemoveAll(crewPath); err != nil {
		return fmt.Errorf("removing crew dir: %w", err)
	}

	return nil
}

// List returns all crew workers in the rig.
func (m *Manager) List() ([]*CrewWorker, error) {
	crewBaseDir := filepath.Join(m.rig.Path, "crew")

	entries, err := os.ReadDir(crewBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading crew dir: %w", err)
	}

	var workers []*CrewWorker
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		worker, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid workers
		}
		workers = append(workers, worker)
	}

	return workers, nil
}

// Get returns a specific crew worker by name.
func (m *Manager) Get(name string) (*CrewWorker, error) {
	if !m.exists(name) {
		return nil, ErrCrewNotFound
	}

	return m.loadState(name)
}

// saveState persists crew worker state to disk.
func (m *Manager) saveState(crew *CrewWorker) error {
	data, err := json.MarshalIndent(crew, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	stateFile := m.stateFile(crew.Name)
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// loadState reads crew worker state from disk.
func (m *Manager) loadState(name string) (*CrewWorker, error) {
	stateFile := m.stateFile(name)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return minimal crew worker if state file missing
			return &CrewWorker{
				Name:      name,
				Rig:       m.rig.Name,
				ClonePath: m.crewDir(name),
			}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var crew CrewWorker
	if err := json.Unmarshal(data, &crew); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	return &crew, nil
}
