package rockets

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
)

var testTime = time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)

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

func mustProcess(t *testing.T, s *Store, event messages.Event) ProcessResult {
	t.Helper()
	result, err := s.Process(event)
	require.NoError(t, err)
	return result
}

func TestProcess_LaunchAppliesAndRocketBecomesVisible(t *testing.T) {
	s := NewStore()

	result := mustProcess(t, s, launch("r1", 1, 500))
	require.Equal(t, ResultApplied, result)

	rocket, found := s.Get("r1")
	require.True(t, found)
	require.Equal(t, "r1", rocket.ID())
	require.Equal(t, "Falcon-9", rocket.Type())
	require.Equal(t, "ARTEMIS", rocket.Mission())
	require.Equal(t, 500, rocket.Speed())
	require.Equal(t, StatusLaunched, rocket.Status())
	require.Equal(t, testTime.Add(time.Second), rocket.LaunchedAt())
	require.Equal(t, testTime.Add(time.Second), rocket.LastUpdatedAt())
}

func TestProcess_InvalidMessageNumbersAreDiscarded(t *testing.T) {
	s := NewStore()

	for _, number := range []int64{0, -1, -100} {
		result := mustProcess(t, s, launch("r1", number, 500))
		require.Equal(t, ResultInvalid, result, "messageNumber %d", number)
	}

	_, found := s.Get("r1")
	require.False(t, found)
	require.Equal(t, 0, s.Count())
}

func TestProcess_OutOfOrderMessagesApplyInMessageNumberOrder(t *testing.T) {
	s := NewStore()

	require.Equal(t, ResultPending, mustProcess(t, s, speedUp("r1", 3, 300)))
	require.Equal(t, ResultApplied, mustProcess(t, s, launch("r1", 1, 500)))
	require.Equal(t, ResultApplied, mustProcess(t, s, speedUp("r1", 2, 200)))

	rocket, found := s.Get("r1")
	require.True(t, found)
	require.Equal(t, 500+200+300, rocket.Speed())
}

func TestProcess_GapStaysPendingUntilFilled(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	require.Equal(t, ResultPending, mustProcess(t, s, speedUp("r1", 3, 300)))

	rocket, _ := s.Get("r1")
	require.Equal(t, 500, rocket.Speed(), "message 3 must wait for message 2")

	require.Equal(t, ResultApplied, mustProcess(t, s, speedUp("r1", 2, 200)))
	rocket, _ = s.Get("r1")
	require.Equal(t, 1000, rocket.Speed(), "filling the gap must flush the buffer")
}

func TestProcess_DuplicateOfAppliedMessageDoesNotMutateState(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	mustProcess(t, s, speedUp("r1", 2, 100))

	require.Equal(t, ResultDuplicate, mustProcess(t, s, speedUp("r1", 2, 100)))
	require.Equal(t, ResultDuplicate, mustProcess(t, s, launch("r1", 1, 500)))

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed())
}

func TestProcess_DuplicateOfPendingMessageStaysBuffered(t *testing.T) {
	s := NewStore()

	require.Equal(t, ResultPending, mustProcess(t, s, speedUp("r1", 2, 100)))
	require.Equal(t, ResultDuplicate, mustProcess(t, s, speedUp("r1", 2, 100)))

	mustProcess(t, s, launch("r1", 1, 500))

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed(), "buffered duplicate must apply exactly once")
}

func TestProcess_ConflictFirstMessageWins(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	mustProcess(t, s, speedUp("r1", 2, 100))

	require.Equal(t, ResultConflict, mustProcess(t, s, speedUp("r1", 2, 999)))
	require.Equal(t, ResultConflict, mustProcess(t, s, speedDown("r1", 2, 100)))

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed())
}

func TestProcess_ConflictOnPendingMessageKeepsFirst(t *testing.T) {
	s := NewStore()

	require.Equal(t, ResultPending, mustProcess(t, s, speedUp("r1", 2, 100)))
	require.Equal(t, ResultConflict, mustProcess(t, s, speedUp("r1", 2, 999)))

	mustProcess(t, s, launch("r1", 1, 500))

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed())
}

func TestProcess_NonLaunchFirstMessageStaysPending(t *testing.T) {
	s := NewStore()

	require.Equal(t, ResultPending, mustProcess(t, s, speedUp("r1", 1, 100)))
	mustProcess(t, s, speedUp("r1", 2, 100))

	_, found := s.Get("r1")
	require.False(t, found, "no state is fabricated while the launch is missing")
}

func TestProcess_SecondLaunchDoesNotResetState(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	mustProcess(t, s, speedUp("r1", 2, 100))

	second := launch("r1", 3, 9000)
	second.Mission = "MARS"
	require.Equal(t, ResultApplied, mustProcess(t, s, second))

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed())
	require.Equal(t, "ARTEMIS", rocket.Mission())
}

func TestProcess_ExplosionIsTerminal(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	mustProcess(t, s, explode("r1", 2, "PRESSURE_VESSEL_FAILURE"))

	require.Equal(t, ResultApplied, mustProcess(t, s, speedUp("r1", 3, 1000)),
		"post-explosion messages are still acknowledged as applied")
	mustProcess(t, s, changeMission("r1", 4, "MARS"))

	rocket, _ := s.Get("r1")
	require.Equal(t, StatusExploded, rocket.Status())
	require.Equal(t, "PRESSURE_VESSEL_FAILURE", rocket.ExplodedReason())
	require.Equal(t, 0, rocket.Speed())
	require.Equal(t, "ARTEMIS", rocket.Mission())
}

func TestProcess_LateMessageBelowExplosionAppliesBeforeIt(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 500))
	require.Equal(t, ResultPending, mustProcess(t, s, explode("r1", 3, "OXYGEN_LEAK")))
	mustProcess(t, s, speedUp("r1", 2, 200))

	rocket, _ := s.Get("r1")
	require.Equal(t, StatusExploded, rocket.Status())
	require.Equal(t, 0, rocket.Speed(), "explosion applies after the late increase and zeroes the speed")
}

func TestProcess_SpeedNeverGoesBelowZero(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 100))
	mustProcess(t, s, speedDown("r1", 2, 500))

	rocket, _ := s.Get("r1")
	require.Equal(t, 0, rocket.Speed())
}

func TestProcess_ChannelsAreIndependent(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 100))
	mustProcess(t, s, launch("r2", 1, 900))
	mustProcess(t, s, speedUp("r1", 2, 50))

	r1, _ := s.Get("r1")
	r2, _ := s.Get("r2")
	require.Equal(t, 150, r1.Speed())
	require.Equal(t, 900, r2.Speed())
}

func TestGet_UnknownChannel(t *testing.T) {
	s := NewStore()

	_, found := s.Get("nonexistent")
	require.False(t, found)
}

func TestGet_ChannelWithoutLaunch(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, speedUp("r1", 2, 100))

	_, found := s.Get("r1")
	require.False(t, found)
}

func TestList_Filters(t *testing.T) {
	s := NewStore()
	for i, speed := range []int{500, 1000, 1500} {
		mustProcess(t, s, launch(fmt.Sprintf("rocket-%c", 'a'+i), 1, speed))
	}

	fast := s.List(ListFilter{MinSpeed: 1000})
	require.Len(t, fast, 2)
	for _, r := range fast {
		require.GreaterOrEqual(t, r.Speed(), 1000)
	}

	slow := s.List(ListFilter{MaxSpeed: 1000})
	require.Len(t, slow, 2)
	for _, r := range slow {
		require.LessOrEqual(t, r.Speed(), 1000)
	}

	band := s.List(ListFilter{MinSpeed: 600, MaxSpeed: 1400})
	require.Len(t, band, 1)
	require.Equal(t, "rocket-b", band[0].ID())

	one := s.List(ListFilter{ID: "rocket-b"})
	require.Len(t, one, 1)
	require.Equal(t, "rocket-b", one[0].ID())

	none := s.List(ListFilter{ID: "rocket-b", MinSpeed: 2000})
	require.Empty(t, none)
}

func TestList_Ordering(t *testing.T) {
	s := NewStore()
	for i, speed := range []int{300, 100, 200} {
		mustProcess(t, s, launch(fmt.Sprintf("rocket-%c", 'a'+i), 1, speed))
	}

	byID := s.List(ListFilter{})
	require.Equal(t, []string{"rocket-a", "rocket-b", "rocket-c"}, ids(byID))

	bySpeed := s.List(ListFilter{OrderBy: "speed"})
	require.Equal(t, []string{"rocket-b", "rocket-c", "rocket-a"}, ids(bySpeed))

	bySpeedDesc := s.List(ListFilter{OrderBy: "speed", OrderDesc: true})
	require.Equal(t, []string{"rocket-a", "rocket-c", "rocket-b"}, ids(bySpeedDesc))
}

func TestList_OrderingByMissionAndStatus(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("rocket-a", 1, 100))
	mustProcess(t, s, launch("rocket-b", 1, 100))
	mustProcess(t, s, changeMission("rocket-a", 2, "ZULU"))
	mustProcess(t, s, changeMission("rocket-b", 2, "ALPHA"))
	mustProcess(t, s, launch("rocket-c", 1, 100))
	mustProcess(t, s, explode("rocket-c", 2, "OXYGEN_LEAK"))

	byMission := s.List(ListFilter{OrderBy: "mission"})
	require.Equal(t, []string{"rocket-b", "rocket-c", "rocket-a"}, ids(byMission))

	byStatus := s.List(ListFilter{OrderBy: "status"})
	require.Equal(t, "rocket-c", byStatus[0].ID(), "exploded sorts before launched")
}

func TestList_TiesBreakByID(t *testing.T) {
	s := NewStore()
	for _, channel := range []string{"rocket-c", "rocket-a", "rocket-b"} {
		mustProcess(t, s, launch(channel, 1, 100))
	}

	bySpeed := s.List(ListFilter{OrderBy: "speed"})
	require.Equal(t, []string{"rocket-a", "rocket-b", "rocket-c"}, ids(bySpeed))
}

func TestList_ExcludesChannelsWithoutLaunch(t *testing.T) {
	s := NewStore()

	mustProcess(t, s, launch("r1", 1, 100))
	mustProcess(t, s, speedUp("pending", 2, 100))

	require.Equal(t, []string{"r1"}, ids(s.GetAll()))
	require.Equal(t, 1, s.Count())
}

func TestGetAllAndCount(t *testing.T) {
	s := NewStore()

	require.Empty(t, s.GetAll())
	require.Equal(t, 0, s.Count())

	mustProcess(t, s, launch("r1", 1, 100))
	mustProcess(t, s, launch("r2", 1, 200))

	require.Len(t, s.GetAll(), 2)
	require.Equal(t, 2, s.Count())
}

func TestProcess_ConcurrentChannelsDoNotInterfere(t *testing.T) {
	s := NewStore()

	const messagesPerChannel = 50
	var wg sync.WaitGroup
	for _, channel := range []string{"chan-a", "chan-b"} {
		wg.Add(1)
		go func(channel string) {
			defer wg.Done()
			_, _ = s.Process(launch(channel, 1, 100))
			for n := int64(2); n <= messagesPerChannel; n++ {
				_, _ = s.Process(speedUp(channel, n, 10))
			}
		}(channel)
	}
	wg.Wait()

	for _, channel := range []string{"chan-a", "chan-b"} {
		rocket, found := s.Get(channel)
		require.True(t, found)
		require.Equal(t, 100+(messagesPerChannel-1)*10, rocket.Speed(), "channel %s", channel)
	}
}

func TestProcess_ConcurrentDuplicatesAreIdempotent(t *testing.T) {
	s := NewStore()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Process(launch("r1", 1, 500))
			_, _ = s.Process(speedUp("r1", 2, 100))
		}()
	}
	wg.Wait()

	rocket, _ := s.Get("r1")
	require.Equal(t, 600, rocket.Speed())
}

func TestStore_ConcurrentReadsDuringWrites(t *testing.T) {
	s := NewStore()

	var wg sync.WaitGroup
	for _, channel := range []string{"r1", "r2", "r3"} {
		wg.Add(1)
		go func(ch string) {
			defer wg.Done()
			_, _ = s.Process(launch(ch, 1, 100))
			for n := int64(2); n <= 100; n++ {
				_, _ = s.Process(speedUp(ch, n, 1))
			}
		}(channel)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.List(ListFilter{OrderBy: "speed"})
				_, _ = s.Get("r1")
				_ = s.Count()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, 3, s.Count())
	rocket, _ := s.Get("r1")
	require.Equal(t, 199, rocket.Speed())
}

func ids(rockets []*Rocket) []string {
	out := make([]string, len(rockets))
	for i, r := range rockets {
		out[i] = r.ID()
	}
	return out
}
