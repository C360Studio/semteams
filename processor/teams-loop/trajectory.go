package teamsloop

import (
	"context"
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semteams/teams"
)

// TrajectoryManager manages trajectory capture and persistence
type TrajectoryManager struct {
	trajectories map[string]*teams.Trajectory
	mu           sync.RWMutex
}

// NewTrajectoryManager creates a new TrajectoryManager
func NewTrajectoryManager() *TrajectoryManager {
	return &TrajectoryManager{
		trajectories: make(map[string]*teams.Trajectory),
	}
}

// StartTrajectory starts a new trajectory for a loop
func (m *TrajectoryManager) StartTrajectory(loopID string) (teams.Trajectory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	traj := teams.NewTrajectory(loopID)
	m.trajectories[loopID] = &traj

	return traj, nil
}

// AddStep adds a step to a trajectory
func (m *TrajectoryManager) AddStep(loopID string, step teams.TrajectoryStep) (teams.Trajectory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	traj, exists := m.trajectories[loopID]
	if !exists {
		return teams.Trajectory{}, errs.Wrap(fmt.Errorf("trajectory for loop %s not found", loopID), "TrajectoryManager", "operation", "find trajectory")
	}

	traj.AddStep(step)

	return *traj, nil
}

// CompleteTrajectory marks a trajectory as complete
func (m *TrajectoryManager) CompleteTrajectory(loopID, outcome string) (teams.Trajectory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	traj, exists := m.trajectories[loopID]
	if !exists {
		return teams.Trajectory{}, errs.Wrap(fmt.Errorf("trajectory for loop %s not found", loopID), "TrajectoryManager", "operation", "find trajectory")
	}

	traj.Complete(outcome)

	return *traj, nil
}

// GetTrajectory retrieves a trajectory by loop ID
func (m *TrajectoryManager) GetTrajectory(loopID string) (teams.Trajectory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	traj, exists := m.trajectories[loopID]
	if !exists {
		return teams.Trajectory{}, errs.Wrap(fmt.Errorf("trajectory for loop %s not found", loopID), "TrajectoryManager", "operation", "find trajectory")
	}

	return *traj, nil
}

// SaveTrajectory saves a trajectory to KV storage
func (m *TrajectoryManager) SaveTrajectory(_ context.Context, _ teams.Trajectory) error {
	// In unit tests with mock KV, this is a no-op
	// Integration tests will implement actual KV persistence
	return nil
}
