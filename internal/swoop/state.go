package swoop

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

const (
	appName  = "kest"
	stateDir = "swoop"
)

// State manages the per-project local state file that tracks when terraform
// actions were last run against each root.
type State struct {
	path    string
	Entries map[string]*StateEntry `yaml:"roots"`
}

// stateBasePath returns $XDG_STATE_HOME/kest/swoop/.
func stateBasePath() string {
	return filepath.Join(xdg.StateHome, appName, stateDir)
}

// projectStateKey returns a stable filename for the given project directory.
// It uses a truncated SHA-256 of the absolute path so different projects
// don't collide.
func projectStateKey(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		abs = projectDir
	}
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h[:8])
}

// LoadState reads the local state file for the given project directory.
// Returns an empty state if the file doesn't exist.
func LoadState(projectDir string) (*State, error) {
	base := stateBasePath()
	path := filepath.Join(base, projectStateKey(projectDir)+".yaml")

	s := &State{
		path:    path,
		Entries: make(map[string]*StateEntry),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing swoop state %s: %w", path, err)
	}
	if s.Entries == nil {
		s.Entries = make(map[string]*StateEntry)
	}
	return s, nil
}

// Save writes the state to disk.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// RecordInit records a terraform init for the given root path.
func (s *State) RecordInit(rootPath string) {
	e := s.entry(rootPath)
	now := time.Now().UTC()
	e.LastInit = &now
}

// RecordPlan records a terraform plan for the given root path.
func (s *State) RecordPlan(rootPath string, result string) {
	e := s.entry(rootPath)
	now := time.Now().UTC()
	e.LastPlan = &now
	e.PlanResult = result
}

// RecordApply records a terraform apply for the given root path.
func (s *State) RecordApply(rootPath string) {
	e := s.entry(rootPath)
	now := time.Now().UTC()
	e.LastApply = &now
}

func (s *State) entry(rootPath string) *StateEntry {
	if e, ok := s.Entries[rootPath]; ok {
		return e
	}
	e := &StateEntry{}
	s.Entries[rootPath] = e
	return e
}
