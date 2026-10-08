package application

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/rockets"
)

var testTime = time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)

func newTestProcessor() *RocketEventProcessor {
	return NewRocketEventProcessor(rockets.NewStore(), zap.NewNop())
}

func metadata(channel string, number int64, messageType messages.MessageType) messages.Metadata {
	return messages.Metadata{
		Channel:       channel,
		MessageNumber: number,
		MessageTime:   testTime.Add(time.Duration(number) * time.Second),
		MessageType:   messageType,
	}
}

func launch(channel string, number int64, speed int) messages.RocketLaunchedEvent {
	return messages.RocketLaunchedEvent{
		Metadata:    metadata(channel, number, messages.TypeRocketLaunched),
		Type:        "Falcon-9",
		LaunchSpeed: speed,
		Mission:     "ARTEMIS",
	}
}

func speedUp(channel string, number int64, by int) messages.RocketSpeedIncreasedEvent {
	return messages.RocketSpeedIncreasedEvent{
		Metadata: metadata(channel, number, messages.TypeRocketSpeedIncreased),
		By:       by,
	}
}

func speedDown(channel string, number int64, by int) messages.RocketSpeedDecreasedEvent {
	return messages.RocketSpeedDecreasedEvent{
		Metadata: metadata(channel, number, messages.TypeRocketSpeedDecreased),
		By:       by,
	}
}

func explode(channel string, number int64, reason string) messages.RocketExplodedEvent {
	return messages.RocketExplodedEvent{
		Metadata: metadata(channel, number, messages.TypeRocketExploded),
		Reason:   reason,
	}
}

func changeMission(channel string, number int64, mission string) messages.RocketMissionChangedEvent {
	return messages.RocketMissionChangedEvent{
		Metadata:   metadata(channel, number, messages.TypeRocketMissionChanged),
		NewMission: mission,
	}
}

// Out-of-order delivery: messages 3, 1, 2 must produce the same state as
// 1, 2, 3 because state is rebuilt in messageNumber order.
func TestProcess_OutOfOrderMessagesApplyInMessageNumberOrder(t *testing.T) {
	p := newTestProcessor()

	result, err := p.Process(speedUp("r1", 3, 300))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultPending, result)

	result, err = p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultApplied, result)

	result, err = p.Process(speedUp("r1", 2, 200))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultApplied, result)

	rocket, found := p.GetRocket("r1")
	require.True(t, found)
	require.Equal(t, 500+200+300, rocket.Speed())
}

// A redelivered copy of an already applied message must not mutate state.
func TestProcess_DuplicateMessageDoesNotApplyTwice(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	_, err = p.Process(speedUp("r1", 2, 100))
	require.NoError(t, err)

	result, err := p.Process(speedUp("r1", 2, 100))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultDuplicate, result)

	result, err = p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultDuplicate, result)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, 600, rocket.Speed())
}

// An event that arrives before the launch is buffered and applied as soon
// as the launch arrives.
func TestProcess_EventBeforeLaunchIsBufferedUntilLaunchArrives(t *testing.T) {
	p := newTestProcessor()

	result, err := p.Process(speedUp("r1", 2, 250))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultPending, result)

	_, found := p.GetRocket("r1")
	require.False(t, found, "rocket must not exist before its launch is applied")

	result, err = p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultApplied, result)

	rocket, found := p.GetRocket("r1")
	require.True(t, found)
	require.Equal(t, 750, rocket.Speed())
}

// If message #1 is not the launch, nothing is fabricated: later messages
// stay pending until the missing number arrives.
func TestProcess_NonLaunchFirstMessageStaysPending(t *testing.T) {
	p := newTestProcessor()

	result, err := p.Process(speedUp("r1", 1, 100))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultPending, result)

	_, err = p.Process(speedUp("r1", 2, 100))
	require.NoError(t, err)

	_, found := p.GetRocket("r1")
	require.False(t, found)
}

// Explosion zeroes the speed; later speed or mission messages are accepted
// but must not change the state.
func TestProcess_ExplosionIsTerminal(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	_, err = p.Process(explode("r1", 2, "PRESSURE_VESSEL_FAILURE"))
	require.NoError(t, err)

	result, err := p.Process(speedUp("r1", 3, 1000))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultApplied, result, "post-explosion messages are still acknowledged as applied")

	_, err = p.Process(changeMission("r1", 4, "MARS"))
	require.NoError(t, err)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, rockets.StatusExploded, rocket.Status())
	require.Equal(t, "PRESSURE_VESSEL_FAILURE", rocket.ExplodedReason())
	require.Equal(t, 0, rocket.Speed())
	require.Equal(t, "ARTEMIS", rocket.Mission())
}

// A late message numbered below the explosion is applied before it, because
// application is contiguous: the explosion only applies once 1..k-1 exist.
func TestProcess_LateMessageBelowExplosionAppliesBeforeIt(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 500))
	require.NoError(t, err)

	// Explosion arrives as #3 while #2 is still missing: it must wait.
	result, err := p.Process(explode("r1", 3, "OXYGEN_LEAK"))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultPending, result)

	_, err = p.Process(speedUp("r1", 2, 200))
	require.NoError(t, err)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, rockets.StatusExploded, rocket.Status())
	require.Equal(t, 0, rocket.Speed(), "explosion zeroes the speed after the late increase was applied")
}

// Two channels processed concurrently must not interfere with each other.
func TestProcess_ConcurrentChannelsDoNotInterfere(t *testing.T) {
	p := newTestProcessor()

	const messagesPerChannel = 50
	var wg sync.WaitGroup
	for _, channel := range []string{"chan-a", "chan-b"} {
		wg.Add(1)
		go func(channel string) {
			defer wg.Done()
			_, err := p.Process(launch(channel, 1, 100))
			require.NoError(t, err)
			for n := int64(2); n <= messagesPerChannel; n++ {
				_, err := p.Process(speedUp(channel, n, 10))
				require.NoError(t, err)
			}
		}(channel)
	}
	wg.Wait()

	for _, channel := range []string{"chan-a", "chan-b"} {
		rocket, found := p.GetRocket(channel)
		require.True(t, found)
		require.Equal(t, 100+(messagesPerChannel-1)*10, rocket.Speed(), "channel %s", channel)
	}
}

// Concurrent duplicate delivery of the same messages must be safe and
// idempotent.
func TestProcess_ConcurrentDuplicatesAreIdempotent(t *testing.T) {
	p := newTestProcessor()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.Process(launch("r1", 1, 500))
			_, _ = p.Process(speedUp("r1", 2, 100))
		}()
	}
	wg.Wait()

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, 600, rocket.Speed())
}

// Same messageNumber with a different payload: the first message wins and
// the conflict is not an error.
func TestProcess_ConflictFirstWins(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	_, err = p.Process(speedUp("r1", 2, 100))
	require.NoError(t, err)

	result, err := p.Process(speedUp("r1", 2, 999))
	require.NoError(t, err)
	require.Equal(t, rockets.ResultConflict, result)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, 600, rocket.Speed())
}

// A duplicate delivered through the public entry point is acknowledged,
// not reported as a domain error (so the caller answers 2xx and the test
// program does not redeliver).
func TestProcess_DuplicateIsNotAnError(t *testing.T) {
	p := newTestProcessor()

	event := launch("r1", 1, 500)
	_, err := p.Process(event)
	require.NoError(t, err)

	result, err := p.Process(event)
	require.NoError(t, err)
	require.Equal(t, rockets.ResultDuplicate, result)
}

// Speed is clamped at 0 (our documented decision, not a requirement).
func TestProcess_SpeedNeverGoesBelowZero(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 100))
	require.NoError(t, err)
	_, err = p.Process(speedDown("r1", 2, 500))
	require.NoError(t, err)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, 0, rocket.Speed())
}

func TestProcess_MissionChangeApplies(t *testing.T) {
	p := newTestProcessor()

	_, err := p.Process(launch("r1", 1, 500))
	require.NoError(t, err)
	_, err = p.Process(changeMission("r1", 2, "MARS"))
	require.NoError(t, err)

	rocket, _ := p.GetRocket("r1")
	require.Equal(t, "MARS", rocket.Mission())
}

func TestGetRocket_NotFound(t *testing.T) {
	p := newTestProcessor()

	_, found := p.GetRocket("nonexistent")
	require.False(t, found)
}

// The list endpoint orders by id ascending by default and accepts
// order_by speed/mission/status with direction.
func TestListRockets_Ordering(t *testing.T) {
	p := newTestProcessor()

	for i, speed := range []int{300, 100, 200} {
		channel := fmt.Sprintf("rocket-%c", 'a'+i)
		_, err := p.Process(launch(channel, 1, speed))
		require.NoError(t, err)
	}

	byID := p.ListRockets(rockets.ListFilter{})
	require.Len(t, byID, 3)
	require.Equal(t, "rocket-a", byID[0].ID())
	require.Equal(t, "rocket-b", byID[1].ID())
	require.Equal(t, "rocket-c", byID[2].ID())

	bySpeed := p.ListRockets(rockets.ListFilter{OrderBy: "speed"})
	require.Equal(t, "rocket-b", bySpeed[0].ID())
	require.Equal(t, "rocket-c", bySpeed[1].ID())
	require.Equal(t, "rocket-a", bySpeed[2].ID())

	bySpeedDesc := p.ListRockets(rockets.ListFilter{OrderBy: "speed", OrderDesc: true})
	require.Equal(t, "rocket-a", bySpeedDesc[0].ID())
	require.Equal(t, "rocket-b", bySpeedDesc[2].ID())
}

func TestListRockets_Filters(t *testing.T) {
	p := newTestProcessor()

	for i, speed := range []int{500, 1000, 1500} {
		channel := fmt.Sprintf("rocket-%c", 'a'+i)
		_, err := p.Process(launch(channel, 1, speed))
		require.NoError(t, err)
	}

	fast := p.ListRockets(rockets.ListFilter{MinSpeed: 1000})
	require.Len(t, fast, 2)
	for _, r := range fast {
		require.GreaterOrEqual(t, r.Speed(), 1000)
	}

	one := p.ListRockets(rockets.ListFilter{ID: "rocket-b"})
	require.Len(t, one, 1)
	require.Equal(t, "rocket-b", one[0].ID())
}
