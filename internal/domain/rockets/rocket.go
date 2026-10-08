package rockets

import (
	"sync"
	"time"
)

// Status represents the current status of a rocket
type Status string

const (
	StatusLaunched Status = "launched"
	StatusExploded Status = "exploded"
)

// Rocket represents the state of a rocket at any given point in time.
// The mutex only guards reads against concurrent writes; write ordering
// is guaranteed by the per-channel lock in the Store.
type Rocket struct {
	mu sync.RWMutex

	id             string
	rocketType     string
	mission        string
	speed          int
	status         Status
	explodedReason string
	launchedAt     time.Time
	lastUpdatedAt  time.Time
}

// NewRocket creates a new rocket from a launch event
func NewRocket(id, rocketType, mission string, launchSpeed int, launchedAt time.Time) *Rocket {
	return &Rocket{
		id:            id,
		rocketType:    rocketType,
		mission:       mission,
		speed:         launchSpeed,
		status:        StatusLaunched,
		launchedAt:    launchedAt,
		lastUpdatedAt: launchedAt,
	}
}

// ID returns the rocket's unique identifier (channel)
func (r *Rocket) ID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.id
}

// Type returns the rocket type (e.g., Falcon-9)
func (r *Rocket) Type() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rocketType
}

// Mission returns the current mission name
func (r *Rocket) Mission() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mission
}

// Speed returns the current speed of the rocket
func (r *Rocket) Speed() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.speed
}

// Status returns the current status of the rocket
func (r *Rocket) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

// ExplodedReason returns the reason for explosion (if exploded)
func (r *Rocket) ExplodedReason() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.explodedReason
}

// LaunchedAt returns the timestamp when the rocket was launched
func (r *Rocket) LaunchedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.launchedAt
}

// LastUpdatedAt returns the timestamp of the last update
func (r *Rocket) LastUpdatedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastUpdatedAt
}

// IncreaseSpeed increases the rocket's speed by the given amount
func (r *Rocket) IncreaseSpeed(by int, eventTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.speed += by
	r.lastUpdatedAt = eventTime
}

// DecreaseSpeed decreases the rocket's speed by the given amount.
// Speed is clamped at 0: a rocket cannot fly backwards (our decision,
// not a requirement of the challenge statement).
func (r *Rocket) DecreaseSpeed(by int, eventTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.speed -= by
	if r.speed < 0 {
		r.speed = 0
	}
	r.lastUpdatedAt = eventTime
}

// Explode marks the rocket as exploded and zeroes its speed
func (r *Rocket) Explode(reason string, eventTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = StatusExploded
	r.explodedReason = reason
	r.speed = 0
	r.lastUpdatedAt = eventTime
}

// ChangeMission changes the rocket's mission
func (r *Rocket) ChangeMission(newMission string, eventTime time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mission = newMission
	r.lastUpdatedAt = eventTime
}
