package rockets

import (
	"sync"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
)

// channelState holds the inbox and the reconstructed state for one channel
// (one rocket). All mutations are serialized by mu, so writers of different
// channels never block each other.
type channelState struct {
	mu sync.Mutex

	id     string
	rocket *Rocket // nil until a RocketLaunched message is applied

	// received holds every accepted message by messageNumber, applied or
	// not. It backs duplicate/conflict detection and the contiguous apply.
	received map[int64]messages.Event

	// lastApplied is the highest contiguous applied message number.
	lastApplied int64

	// explodedAt is the messageNumber of the applied explosion (0 = not exploded).
	explodedAt int64
}

func newChannelState(id string) *channelState {
	return &channelState{
		id:       id,
		received: make(map[int64]messages.Event),
	}
}

func (c *channelState) process(event messages.Event) (ProcessResult, error) {
	md := event.GetMetadata()

	c.mu.Lock()
	defer c.mu.Unlock()

	if md.MessageNumber < 1 {
		return ResultInvalid, nil
	}

	if prev, ok := c.received[md.MessageNumber]; ok {
		if messages.Fingerprint(prev) == messages.Fingerprint(event) {
			return ResultDuplicate, nil
		}
		// The statement gives no criterion to pick a winner, so the first
		// message received wins and the conflicting one is discarded.
		return ResultConflict, nil
	}

	c.received[md.MessageNumber] = event
	c.applyContiguous()

	if md.MessageNumber <= c.lastApplied {
		return ResultApplied, nil
	}
	return ResultPending, nil
}

// current returns the rocket state, or nil if the launch has not been applied.
func (c *channelState) current() *Rocket {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rocket
}

// applyContiguous applies buffered messages while the next contiguous
// messageNumber is available and applicable.
func (c *channelState) applyContiguous() {
	for {
		next, ok := c.received[c.lastApplied+1]
		if !ok {
			return
		}
		if !c.apply(next) {
			return
		}
		c.lastApplied++
	}
}

// apply applies a single message, reporting whether the contiguous sequence
// may advance. It returns false only when the launch is still missing and
// the next message is not a launch: the launch is expected to be message #1,
// and we do not fabricate one — later messages stay pending until it arrives.
func (c *channelState) apply(event messages.Event) bool {
	md := event.GetMetadata()

	if c.rocket == nil {
		launch, ok := event.(messages.RocketLaunchedEvent)
		if !ok {
			return false
		}
		c.rocket = NewRocket(c.id, launch.Type, launch.Mission, launch.LaunchSpeed, md.MessageTime)
		return true
	}

	// A second launch for the same channel does not reset state.
	if _, isLaunch := event.(messages.RocketLaunchedEvent); isLaunch {
		return true
	}

	// After the explosion, later messages are accepted so the sequence
	// advances, but they must not mutate speed or mission. Because messages
	// are applied in contiguous order, everything numbered below the
	// explosion was necessarily applied before it.
	if c.explodedAt > 0 {
		return true
	}

	switch e := event.(type) {
	case messages.RocketSpeedIncreasedEvent:
		c.rocket.IncreaseSpeed(e.By, md.MessageTime)
	case messages.RocketSpeedDecreasedEvent:
		c.rocket.DecreaseSpeed(e.By, md.MessageTime)
	case messages.RocketExplodedEvent:
		c.rocket.Explode(e.Reason, md.MessageTime)
		c.explodedAt = md.MessageNumber
	case messages.RocketMissionChangedEvent:
		c.rocket.ChangeMission(e.NewMission, md.MessageTime)
	}
	return true
}
