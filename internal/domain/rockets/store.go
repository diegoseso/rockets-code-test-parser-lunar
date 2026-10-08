package rockets

import (
	"sort"
	"sync"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
)

// ProcessResult describes the outcome of submitting a message to the store.
type ProcessResult string

const (
	ResultApplied   ProcessResult = "applied"   // applied to the rocket state
	ResultPending   ProcessResult = "pending"   // buffered: earlier message numbers are missing
	ResultDuplicate ProcessResult = "duplicate" // same messageNumber and payload already received
	ResultConflict  ProcessResult = "conflict"  // same messageNumber, different payload: first wins
	ResultInvalid   ProcessResult = "invalid"   // can never be applied (e.g. messageNumber < 1)
)

// Store is the single source of truth for rocket state: one inbox per
// channel, applied in contiguous messageNumber order. Writes are
// serialized per channel, never globally. Safe for concurrent use.
type Store struct {
	mu       sync.RWMutex // guards the channels map only
	channels map[string]*channelState
}

func NewStore() *Store {
	return &Store{
		channels: make(map[string]*channelState),
	}
}

// Process submits one event to its channel inbox and applies everything
// that can be applied in contiguous order. Duplicates, conflicts, pending
// and invalid messages are not errors.
func (s *Store) Process(event messages.Event) (ProcessResult, error) {
	return s.channelFor(event.GetChannel()).process(event)
}

func (s *Store) channelFor(id string) *channelState {
	s.mu.RLock()
	ch, ok := s.channels[id]
	s.mu.RUnlock()
	if ok {
		return ch
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.channels[id]; ok {
		return ch
	}
	ch = newChannelState(id)
	s.channels[id] = ch
	return ch
}

// Get returns the current state of a rocket, or false when the channel
// is unknown or its launch has not been applied yet.
func (s *Store) Get(id string) (*Rocket, bool) {
	s.mu.RLock()
	ch, ok := s.channels[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	rocket := ch.current()
	if rocket == nil {
		return nil, false
	}
	return rocket, true
}

type ListFilter struct {
	ID        string // exact match
	MinSpeed  int
	MaxSpeed  int    // 0 means no limit
	OrderBy   string // "id" (default), "speed", "mission", "status"
	OrderDesc bool
}

// List returns all launched rockets matching the filter, stably ordered
// with ties broken by ID so the order is deterministic.
func (s *Store) List(filter ListFilter) []*Rocket {
	s.mu.RLock()
	channels := make([]*channelState, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}
	s.mu.RUnlock()

	result := make([]*Rocket, 0, len(channels))
	for _, ch := range channels {
		rocket := ch.current()
		if rocket == nil {
			continue
		}
		if filter.ID != "" && rocket.ID() != filter.ID {
			continue
		}
		speed := rocket.Speed()
		if filter.MinSpeed > 0 && speed < filter.MinSpeed {
			continue
		}
		if filter.MaxSpeed > 0 && speed > filter.MaxSpeed {
			continue
		}
		result = append(result, rocket)
	}

	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch filter.OrderBy {
		case "speed":
			if a.Speed() != b.Speed() {
				return a.Speed() < b.Speed()
			}
		case "mission":
			if a.Mission() != b.Mission() {
				return a.Mission() < b.Mission()
			}
		case "status":
			if a.Status() != b.Status() {
				return a.Status() < b.Status()
			}
		}
		return a.ID() < b.ID()
	})

	if filter.OrderDesc {
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
	}

	return result
}

func (s *Store) GetAll() []*Rocket {
	return s.List(ListFilter{})
}

func (s *Store) Count() int {
	return len(s.List(ListFilter{}))
}
