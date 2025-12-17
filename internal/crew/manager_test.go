package crew

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

func TestManagerAddAndGet(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "crew-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock rig
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	// Initialize git repo for the rig
	g := git.NewGit(rigPath)

	// For testing, we need a git URL - use a local bare repo
	bareRepoPath := filepath.Join(tmpDir, "bare-repo.git")
	cmd := []string{"git", "init", "--bare", bareRepoPath}
	if err := runCmd(cmd[0], cmd[1:]...); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	r := &rig.Rig{
		Name:   "test-rig",
		Path:   rigPath,
		GitURL: bareRepoPath,
	}

	mgr := NewManager(r, g)

	// Test Add
	worker, err := mgr.Add("dave", false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if worker.Name != "dave" {
		t.Errorf("expected name 'dave', got '%s'", worker.Name)
	}
	if worker.Rig != "test-rig" {
		t.Errorf("expected rig 'test-rig', got '%s'", worker.Rig)
	}
	if worker.Branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", worker.Branch)
	}

	// Verify directory structure
	crewDir := filepath.Join(rigPath, "crew", "dave")
	if _, err := os.Stat(crewDir); os.IsNotExist(err) {
		t.Error("crew directory was not created")
	}

	mailDir := filepath.Join(crewDir, "mail")
	if _, err := os.Stat(mailDir); os.IsNotExist(err) {
		t.Error("mail directory was not created")
	}

	claudeMD := filepath.Join(crewDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
		t.Error("CLAUDE.md was not created")
	}

	stateFile := filepath.Join(crewDir, "state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("state.json was not created")
	}

	// Test Get
	retrieved, err := mgr.Get("dave")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "dave" {
		t.Errorf("expected name 'dave', got '%s'", retrieved.Name)
	}

	// Test duplicate Add
	_, err = mgr.Add("dave", false)
	if err != ErrCrewExists {
		t.Errorf("expected ErrCrewExists, got %v", err)
	}

	// Test Get non-existent
	_, err = mgr.Get("nonexistent")
	if err != ErrCrewNotFound {
		t.Errorf("expected ErrCrewNotFound, got %v", err)
	}
}

func TestManagerAddWithBranch(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "crew-test-branch-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock rig
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	g := git.NewGit(rigPath)

	// Create a local repo with initial commit for branch testing
	sourceRepoPath := filepath.Join(tmpDir, "source-repo")
	if err := os.MkdirAll(sourceRepoPath, 0755); err != nil {
		t.Fatalf("failed to create source repo dir: %v", err)
	}

	// Initialize source repo with a commit
	cmds := [][]string{
		{"git", "-C", sourceRepoPath, "init"},
		{"git", "-C", sourceRepoPath, "config", "user.email", "test@test.com"},
		{"git", "-C", sourceRepoPath, "config", "user.name", "Test"},
	}
	for _, cmd := range cmds {
		if err := runCmd(cmd[0], cmd[1:]...); err != nil {
			t.Fatalf("failed to run %v: %v", cmd, err)
		}
	}

	// Create initial file and commit
	testFile := filepath.Join(sourceRepoPath, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmds = [][]string{
		{"git", "-C", sourceRepoPath, "add", "."},
		{"git", "-C", sourceRepoPath, "commit", "-m", "Initial commit"},
	}
	for _, cmd := range cmds {
		if err := runCmd(cmd[0], cmd[1:]...); err != nil {
			t.Fatalf("failed to run %v: %v", cmd, err)
		}
	}

	r := &rig.Rig{
		Name:   "test-rig",
		Path:   rigPath,
		GitURL: sourceRepoPath,
	}

	mgr := NewManager(r, g)

	// Test Add with branch
	worker, err := mgr.Add("emma", true)
	if err != nil {
		t.Fatalf("Add with branch failed: %v", err)
	}

	if worker.Branch != "crew/emma" {
		t.Errorf("expected branch 'crew/emma', got '%s'", worker.Branch)
	}
}

func TestManagerList(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "crew-test-list-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock rig
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	g := git.NewGit(rigPath)

	// Create a bare repo for cloning
	bareRepoPath := filepath.Join(tmpDir, "bare-repo.git")
	if err := runCmd("git", "init", "--bare", bareRepoPath); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	r := &rig.Rig{
		Name:   "test-rig",
		Path:   rigPath,
		GitURL: bareRepoPath,
	}

	mgr := NewManager(r, g)

	// Initially empty
	workers, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(workers) != 0 {
		t.Errorf("expected 0 workers, got %d", len(workers))
	}

	// Add some workers
	_, err = mgr.Add("alice", false)
	if err != nil {
		t.Fatalf("Add alice failed: %v", err)
	}
	_, err = mgr.Add("bob", false)
	if err != nil {
		t.Fatalf("Add bob failed: %v", err)
	}

	workers, err = mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(workers))
	}
}

func TestManagerRemove(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "crew-test-remove-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock rig
	rigPath := filepath.Join(tmpDir, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("failed to create rig dir: %v", err)
	}

	g := git.NewGit(rigPath)

	// Create a bare repo for cloning
	bareRepoPath := filepath.Join(tmpDir, "bare-repo.git")
	if err := runCmd("git", "init", "--bare", bareRepoPath); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}

	r := &rig.Rig{
		Name:   "test-rig",
		Path:   rigPath,
		GitURL: bareRepoPath,
	}

	mgr := NewManager(r, g)

	// Add a worker
	_, err = mgr.Add("charlie", false)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Remove it (with force since CLAUDE.md is uncommitted)
	err = mgr.Remove("charlie", true)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify it's gone
	_, err = mgr.Get("charlie")
	if err != ErrCrewNotFound {
		t.Errorf("expected ErrCrewNotFound, got %v", err)
	}

	// Remove non-existent
	err = mgr.Remove("nonexistent", false)
	if err != ErrCrewNotFound {
		t.Errorf("expected ErrCrewNotFound, got %v", err)
	}
}

// Helper to run commands
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}
