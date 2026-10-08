package application

import (
	"go.uber.org/zap"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/rockets"
)

// RocketEventProcessor applies rocket events to the store. It is the only
// entry point for writes and shares its store with the read side.
type RocketEventProcessor struct {
	store  *rockets.Store
	logger *zap.Logger
}

// NewRocketEventProcessor creates a new processor backed by the given store.
func NewRocketEventProcessor(store *rockets.Store, logger *zap.Logger) *RocketEventProcessor {
	return &RocketEventProcessor{
		store:  store,
		logger: logger,
	}
}

// Process submits one event to the store.
//
// A non-nil error means the failure is transient and the caller should ask
// for redelivery (non-2xx). Duplicates, conflicts, buffered (pending) and
// permanently invalid messages are reported through the ProcessResult and
// are deliberately not errors: redelivering them would never succeed.
func (p *RocketEventProcessor) Process(event messages.Event) (rockets.ProcessResult, error) {
	md := event.GetMetadata()
	log := p.logger.With(
		zap.String("channel", md.Channel),
		zap.Int64("message_number", md.MessageNumber),
		zap.String("message_type", string(md.MessageType)),
	)

	result, err := p.store.Process(event)
	if err != nil {
		log.Error("transient failure processing message, will be redelivered", zap.Error(err))
		return result, err
	}

	switch result {
	case rockets.ResultDuplicate:
		log.Debug("duplicate message, state untouched")
	case rockets.ResultConflict:
		log.Warn("message number already seen with a different payload, first one wins",
			zap.String("fingerprint", messages.Fingerprint(event)))
	case rockets.ResultPending:
		log.Debug("message buffered, waiting for earlier message numbers")
	case rockets.ResultInvalid:
		log.Warn("invalid message discarded", zap.String("message_time_raw", md.MessageTimeRaw))
	}

	return result, nil
}

// GetRocket retrieves a rocket by ID
func (p *RocketEventProcessor) GetRocket(id string) (*rockets.Rocket, bool) {
	return p.store.Get(id)
}

// ListRockets returns rockets based on filter criteria
func (p *RocketEventProcessor) ListRockets(filter rockets.ListFilter) []*rockets.Rocket {
	return p.store.List(filter)
}
