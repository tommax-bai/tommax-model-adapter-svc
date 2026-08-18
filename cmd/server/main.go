// model-adapter-svc 入口：装配 provider 注册表 → jobstore → gRPC server。
// Phase 1 手动装配（wire 引入为规模化后统一项，见 README 例外登记）。
package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/tommax-bai/tommax-go-kit/configx"
	"github.com/tommax-bai/tommax-go-kit/logx"
	modeladapterv1 "github.com/tommax-bai/tommax-proto/gen/go/tommax/modeladapter/v1"

	"github.com/tommax-bai/tommax-model-adapter-svc/internal/conf"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/jobstore"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/provider/mock"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/router"
	"github.com/tommax-bai/tommax-model-adapter-svc/internal/server"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	var cfg conf.Config
	if err := configx.Load(*configPath, &cfg); err != nil {
		panic(err)
	}
	log := logx.Init("model-adapter-svc", cfg.Log.Level, cfg.Log.Format)

	jobs := jobstore.New(time.Duration(cfg.Job.TTLMinutes) * time.Minute)
	rt := router.New(
		mock.New(time.Duration(cfg.Mock.LatencyMs) * time.Millisecond),
		// 新供应商在此注册：seedance.New(...), kling.New(...), ...
	)

	lis, err := net.Listen("tcp", cfg.Server.GRPCAddr)
	if err != nil {
		log.Error("listen failed", "addr", cfg.Server.GRPCAddr, "err", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	modeladapterv1.RegisterInferenceServiceServer(grpcServer, server.NewInferenceServer(rt, jobs))

	go func() {
		log.Info("grpc listening", "addr", cfg.Server.GRPCAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("grpc serve exit", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")
	grpcServer.GracefulStop()
}
