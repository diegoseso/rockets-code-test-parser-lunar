package internal

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	runtimeGRPC "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/adapters/messaging"
	rocketsAdapter "github.com/diegoseso/rockets-code-test-parser-lunar/internal/adapters/rockets"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/application"
	"github.com/diegoseso/rockets-code-test-parser-lunar/internal/domain/rockets"
	"github.com/diegoseso/rockets-code-test-parser-lunar/pkg/config"
	messagesAPI "github.com/diegoseso/rockets-code-test-parser-lunar/proto/gen/proto/go/gateway/messages/v1"
	rocketsAPI "github.com/diegoseso/rockets-code-test-parser-lunar/proto/gen/proto/go/gateway/rockets/v1"
)

var Name = "rockets_gateway"

func Bootstrap(ctx context.Context, cfg config.Values, logger *zap.Logger) error {
	logger.Info("starting rockets gateway service")

	be, err := newApplication(ctx, cfg, logger)
	if err != nil {
		return errors.New("failed to initialize application: " + err.Error())
	}

	g, errGroupContext := errgroup.WithContext(ctx)
	starter := func(name string, f func(context.Context) error) {
		g.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					be.ErrHandler(errors.New("panic on "+name), r)
				}
			}()
			return errors.Wrapf(f(errGroupContext), "failed to start %s", name)
		})
	}

	// A single store backs both the write side (messages API) and the read
	// side (rockets API), so they can never diverge.
	store := rockets.NewStore()
	processor := application.NewRocketEventProcessor(store, logger)

	starter("grpc-server", func(ctx context.Context) error {
		return be.serveGrpc(ctx, cfg, processor, logger)
	})

	starter("https-server", func(ctx context.Context) error {
		return serveHTTP(ctx, cfg)
	})

	return g.Wait()
}

func newApplication(_ context.Context, _ config.Values, _ *zap.Logger) (Application, error) {
	return Application{}, nil
}

type Application struct {
	ErrHandler func(error, interface{})
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Printf("gRPC method: %s, Payload: %+v", info.FullMethod, req)
	return handler(ctx, req)
}

func (bo *Application) serveGrpc(ctx context.Context, cfg config.Values, processor *application.RocketEventProcessor, logger *zap.Logger) error {
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(loggingInterceptor))

	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	listener, err := startGRPCListener(&cfg)

	if err != nil {
		return errors.Wrap(err, "Unable to initialize listener")
	}

	rocketsAPI.RegisterRocketsAPIServiceServer(grpcServer, rocketsAdapter.NewServer(processor))
	messagesAPI.RegisterMessagesAPIServiceServer(grpcServer, messaging.NewServer(processor, logger))

	return grpcServer.Serve(listener)
}

func startGRPCListener(cfg *config.Values) (net.Listener, error) {
	l, err := net.Listen("tcp", cfg.GrpcServer)
	if err != nil {
		return nil, err
	}
	cfg.GrpcServer = l.Addr().String()
	return l, nil
}

func serveHTTP(ctx context.Context, cfg config.Values) error {

	rmux := runtimeGRPC.NewServeMux()
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	err := rocketsAPI.RegisterRocketsAPIServiceHandlerFromEndpoint(ctx, rmux, cfg.GrpcServer, opts)
	if err != nil {
		return err
	}

	err = messagesAPI.RegisterMessagesAPIServiceHandlerFromEndpoint(ctx, rmux, cfg.GrpcServer, opts)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", enableCors(rmux))
	server := &http.Server{
		Addr:    cfg.HTTP,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		err := server.Shutdown(context.Background())
		if err != nil {
			return
		}
	}()

	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("HTTP server stopped with error: %v\n", err)
		return err
	}
	fmt.Printf("HTTP server stopped\n")
	return nil
}

func enableCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
