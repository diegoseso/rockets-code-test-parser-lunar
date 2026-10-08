package rockets

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/application"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/rockets"
	rocketsGRPC "github.com/diegoseso/rockets-code-test-parser-lunar/proto/gen/proto/go/gateway/rockets/v1"
)

type adapter struct {
	processor *application.RocketEventProcessor
}

// NewServer creates a new rockets API server
func NewServer(processor *application.RocketEventProcessor) rocketsGRPC.RocketsAPIServiceServer {
	return &adapter{
		processor: processor,
	}
}

// Get retrieves a specific rocket by ID
func (a *adapter) Get(_ context.Context, req *rocketsGRPC.GetRequest) (*rocketsGRPC.GetResponse, error) {
	rocket, found := a.processor.GetRocket(req.Id)
	if !found {
		return nil, status.Error(codes.NotFound, "rocket not found")
	}

	return &rocketsGRPC.GetResponse{
		Rocket: toProto(rocket),
	}, nil
}

// List returns all rockets with optional filtering and sorting.
// Default order is id ascending; order="desc" reverses it.
func (a *adapter) List(_ context.Context, req *rocketsGRPC.ListRequest) (*rocketsGRPC.ListResponse, error) {
	filter := rockets.ListFilter{
		OrderBy:   req.OrderBy,
		OrderDesc: strings.ToLower(req.Order) == "desc",
	}

	if req.FilterId != "" {
		filter.ID = req.FilterId
	}
	if req.FilterMinSpeed > 0 {
		filter.MinSpeed = int(req.FilterMinSpeed)
	}
	if req.FilterMaxSpeed > 0 {
		filter.MaxSpeed = int(req.FilterMaxSpeed)
	}

	rocketsList := a.processor.ListRockets(filter)

	protoRockets := make([]*rocketsGRPC.Rocket, len(rocketsList))
	for i, rocket := range rocketsList {
		protoRockets[i] = toProto(rocket)
	}

	return &rocketsGRPC.ListResponse{
		Rockets: protoRockets,
		Total:   int32(len(protoRockets)),
	}, nil
}

func toProto(rocket *rockets.Rocket) *rocketsGRPC.Rocket {
	out := &rocketsGRPC.Rocket{
		Id:            rocket.ID(),
		Type:          rocket.Type(),
		Mission:       rocket.Mission(),
		Speed:         int32(rocket.Speed()),
		Status:        string(rocket.Status()),
		LaunchedAt:    timestamppb.New(rocket.LaunchedAt()),
		LastUpdatedAt: timestamppb.New(rocket.LastUpdatedAt()),
	}

	if rocket.Status() == rockets.StatusExploded {
		reason := rocket.ExplodedReason()
		out.ExplodedReason = &reason
	}

	return out
}
