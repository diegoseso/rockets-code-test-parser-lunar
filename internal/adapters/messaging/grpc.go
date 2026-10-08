package messaging

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/application"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/messages"
	messagingGRPC "github.com/diegoseso/rockets-code-test-parser-lunar/proto/gen/proto/go/gateway/messages/v1"
)

type adapter struct {
	processor *application.RocketEventProcessor
	logger    *zap.Logger
}

// NewServer creates a new messages API server
func NewServer(processor *application.RocketEventProcessor, logger *zap.Logger) messagingGRPC.MessagesAPIServiceServer {
	return &adapter{
		processor: processor,
		logger:    logger,
	}
}

// Process handles incoming rocket event messages.
//
// Delivery is at-least-once and the test program redelivers on any non-2xx
// response, so the status mapping is part of the contract:
//   - applied, duplicate, conflict, buffered and permanently invalid
//     messages all return 2xx (the message will never be useful again);
//   - only a transient failure returns an error, so the message is
//     redelivered.
func (a *adapter) Process(_ context.Context, req *messagingGRPC.ProcessRequest) (*messagingGRPC.ProcessResponse, error) {
	event, ok := a.buildEvent(req)
	if !ok {
		// Permanently invalid: acknowledge and discard instead of asking
		// for a redelivery that could never succeed.
		return &messagingGRPC.ProcessResponse{}, nil
	}

	if _, err := a.processor.Process(event); err != nil {
		return nil, status.Error(codes.Internal, "temporary failure, safe to redeliver")
	}

	return &messagingGRPC.ProcessResponse{}, nil
}

// buildEvent validates the request and builds the typed domain event.
// It reports false for messages that can never be applied.
func (a *adapter) buildEvent(req *messagingGRPC.ProcessRequest) (messages.Event, bool) {
	if req.Metadata == nil {
		a.logger.Warn("discarding message without metadata")
		return nil, false
	}

	md := req.Metadata
	log := a.logger.With(
		zap.String("channel", md.GetChannel()),
		zap.Int64("message_number", md.GetMessageNumber()),
		zap.String("message_type", md.GetMessageType()),
	)

	if md.GetChannel() == "" {
		log.Warn("discarding message with empty channel")
		return nil, false
	}
	if md.GetMessageNumber() < 1 {
		log.Warn("discarding message with message_number < 1")
		return nil, false
	}

	messageType := messages.MessageType(md.GetMessageType())
	if !messageType.IsValid() {
		log.Warn("discarding message with unknown type")
		return nil, false
	}
	if req.Message == nil {
		log.Warn("discarding message without payload")
		return nil, false
	}

	metadata := messages.Metadata{
		Channel:        md.GetChannel(),
		MessageNumber:  md.GetMessageNumber(),
		MessageTime:    messages.ParseTime(md.GetMessageTime()),
		MessageTimeRaw: md.GetMessageTime(),
		MessageType:    messageType,
	}
	if metadata.MessageTime.IsZero() {
		log.Warn("unparseable message_time, keeping raw value and zero time",
			zap.String("message_time_raw", md.GetMessageTime()))
	}

	msg := req.Message
	switch messageType {
	case messages.TypeRocketLaunched:
		return messages.RocketLaunchedEvent{
			Metadata:    metadata,
			Type:        msg.GetType(),
			LaunchSpeed: int(msg.GetLaunchSpeed()),
			Mission:     msg.GetMission(),
		}, true
	case messages.TypeRocketSpeedIncreased:
		return messages.RocketSpeedIncreasedEvent{Metadata: metadata, By: int(msg.GetBy())}, true
	case messages.TypeRocketSpeedDecreased:
		return messages.RocketSpeedDecreasedEvent{Metadata: metadata, By: int(msg.GetBy())}, true
	case messages.TypeRocketExploded:
		return messages.RocketExplodedEvent{Metadata: metadata, Reason: msg.GetReason()}, true
	case messages.TypeRocketMissionChanged:
		return messages.RocketMissionChangedEvent{Metadata: metadata, NewMission: msg.GetNewMission()}, true
	}

	// Unreachable: the type was validated above.
	return nil, false
}
