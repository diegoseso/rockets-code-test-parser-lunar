package messages

import (
	"fmt"
	"time"
)

// MessageType represents the type of rocket event
type MessageType string

const (
	TypeRocketLaunched       MessageType = "RocketLaunched"
	TypeRocketSpeedIncreased MessageType = "RocketSpeedIncreased"
	TypeRocketSpeedDecreased MessageType = "RocketSpeedDecreased"
	TypeRocketExploded       MessageType = "RocketExploded"
	TypeRocketMissionChanged MessageType = "RocketMissionChanged"
)

// IsValid reports whether the message type is one of the five known types.
func (t MessageType) IsValid() bool {
	switch t {
	case TypeRocketLaunched, TypeRocketSpeedIncreased, TypeRocketSpeedDecreased,
		TypeRocketExploded, TypeRocketMissionChanged:
		return true
	}
	return false
}

// Metadata contains the common metadata for all messages
type Metadata struct {
	Channel       string
	MessageNumber int64
	// MessageTime is the parsed event time. It is the zero Time when
	// MessageTimeRaw could not be parsed; it is never replaced by "now",
	// because a fabricated timestamp would corrupt ordering information.
	MessageTime    time.Time
	MessageTimeRaw string
	MessageType    MessageType
}

// ParseTime parses the messageTime field. The test program emits RFC3339
// timestamps with a variable-length fractional second (e.g.
// "2022-02-02T19:39:05.86337+01:00"), so both RFC3339 and RFC3339Nano are
// accepted. On failure it returns the zero Time; callers keep the raw string.
func ParseTime(raw string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Time{}
}

// RocketLaunchedEvent represents a rocket launch event
type RocketLaunchedEvent struct {
	Metadata    Metadata
	Type        string // Rocket type (e.g., Falcon-9)
	LaunchSpeed int
	Mission     string
}

// RocketSpeedIncreasedEvent represents a speed increase event
type RocketSpeedIncreasedEvent struct {
	Metadata Metadata
	By       int
}

// RocketSpeedDecreasedEvent represents a speed decrease event
type RocketSpeedDecreasedEvent struct {
	Metadata Metadata
	By       int
}

// RocketExplodedEvent represents a rocket explosion event
type RocketExplodedEvent struct {
	Metadata Metadata
	Reason   string
}

// RocketMissionChangedEvent represents a mission change event
type RocketMissionChangedEvent struct {
	Metadata   Metadata
	NewMission string
}

// Event is an interface for all rocket events
type Event interface {
	GetMetadata() Metadata
	GetChannel() string
}

// GetMetadata Implement Event interface for all events
func (e RocketLaunchedEvent) GetMetadata() Metadata       { return e.Metadata }
func (e RocketLaunchedEvent) GetChannel() string          { return e.Metadata.Channel }
func (e RocketSpeedIncreasedEvent) GetMetadata() Metadata { return e.Metadata }
func (e RocketSpeedIncreasedEvent) GetChannel() string    { return e.Metadata.Channel }
func (e RocketSpeedDecreasedEvent) GetMetadata() Metadata { return e.Metadata }
func (e RocketSpeedDecreasedEvent) GetChannel() string    { return e.Metadata.Channel }
func (e RocketExplodedEvent) GetMetadata() Metadata       { return e.Metadata }
func (e RocketExplodedEvent) GetChannel() string          { return e.Metadata.Channel }
func (e RocketMissionChangedEvent) GetMetadata() Metadata { return e.Metadata }
func (e RocketMissionChangedEvent) GetChannel() string    { return e.Metadata.Channel }

// Fingerprint identifies a message by type and payload, ignoring metadata.
func Fingerprint(e Event) string {
	switch v := e.(type) {
	case RocketLaunchedEvent:
		return fmt.Sprintf("RocketLaunched|type=%s|speed=%d|mission=%s", v.Type, v.LaunchSpeed, v.Mission)
	case RocketSpeedIncreasedEvent:
		return fmt.Sprintf("RocketSpeedIncreased|by=%d", v.By)
	case RocketSpeedDecreasedEvent:
		return fmt.Sprintf("RocketSpeedDecreased|by=%d", v.By)
	case RocketExplodedEvent:
		return fmt.Sprintf("RocketExploded|reason=%s", v.Reason)
	case RocketMissionChangedEvent:
		return fmt.Sprintf("RocketMissionChanged|newMission=%s", v.NewMission)
	default:
		return fmt.Sprintf("unknown|%T", e)
	}
}
