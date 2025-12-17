package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Crew command flags
var (
	crewRig    string
	crewBranch bool
)

var crewCmd = &cobra.Command{
	Use:   "crew",
	Short: "Manage crew workspaces (user-managed persistent workspaces)",
	Long: `Crew workers are user-managed persistent workspaces within a rig.

Unlike polecats which are witness-managed and ephemeral, crew workers are:
- Persistent: Not auto-garbage-collected
- User-managed: Overseer controls lifecycle
- Long-lived identities: recognizable names like dave, emma, fred
- Gas Town integrated: Mail, handoff mechanics work
- Tmux optional: Can work in terminal directly

Commands:
  gt crew add <name>      Create a new crew workspace
  gt crew list            List crew workspaces
  gt crew remove <name>   Remove a crew workspace`,
}

var crewAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new crew workspace",
	Long: `Create a new crew workspace with a clone of the rig repository.

The workspace is created at <rig>/crew/<name>/ with:
- A full git clone of the project repository
- Mail directory for message delivery
- CLAUDE.md with crew worker prompting
- Optional feature branch (crew/<name>)

Examples:
  gt crew add dave                    # Create in current rig
  gt crew add emma --rig gastown      # Create in specific rig
  gt crew add fred --branch           # Create with feature branch`,
	Args: cobra.ExactArgs(1),
	RunE: runCrewAdd,
}

func init() {
	crewAddCmd.Flags().StringVar(&crewRig, "rig", "", "Rig to create crew workspace in")
	crewAddCmd.Flags().BoolVar(&crewBranch, "branch", false, "Create a feature branch (crew/<name>)")

	crewCmd.AddCommand(crewAddCmd)
	rootCmd.AddCommand(crewCmd)
}

func runCrewAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "config", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Determine rig
	rigName := crewRig
	if rigName == "" {
		// Try to infer from cwd
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	// Get rig
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Create crew manager
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	// Create crew workspace
	fmt.Printf("Creating crew workspace %s in %s...\n", name, rigName)

	worker, err := crewMgr.Add(name, crewBranch)
	if err != nil {
		if err == crew.ErrCrewExists {
			return fmt.Errorf("crew workspace '%s' already exists", name)
		}
		return fmt.Errorf("creating crew workspace: %w", err)
	}

	fmt.Printf("%s Created crew workspace: %s/%s\n",
		style.Bold.Render("✓"), rigName, name)
	fmt.Printf("  Path: %s\n", worker.ClonePath)
	fmt.Printf("  Branch: %s\n", worker.Branch)
	fmt.Printf("  Mail: %s/mail/\n", worker.ClonePath)

	fmt.Printf("\n%s\n", style.Dim.Render("Start working with: cd "+worker.ClonePath))

	return nil
}

// inferRigFromCwd tries to determine the rig from the current directory.
func inferRigFromCwd(townRoot string) (string, error) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}

	// Check if cwd is within a rig
	rel, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return "", fmt.Errorf("not in workspace")
	}

	// First component should be the rig name
	parts := filepath.SplitList(rel)
	if len(parts) == 0 {
		// Split on path separator instead
		for i := 0; i < len(rel); i++ {
			if rel[i] == filepath.Separator {
				return rel[:i], nil
			}
		}
		// No separator found, entire rel is the rig name
		if rel != "" && rel != "." {
			return rel, nil
		}
	}

	return "", fmt.Errorf("could not infer rig from current directory")
}
