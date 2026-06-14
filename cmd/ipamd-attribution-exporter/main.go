package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/attribution"
	awsec2 "github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/aws"
	appconfig "github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/config"
	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/kube"
	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := appconfig.FromEnv()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	kubeConfig, err := kubernetesConfig()
	if err != nil {
		logger.Error("create kubernetes config", "error", err)
		os.Exit(1)
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		logger.Error("create kubernetes client", "error", err)
		os.Exit(1)
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.AWSRegion))
	if err != nil {
		logger.Error("load aws config", "error", err)
		os.Exit(1)
	}

	reconciler := attribution.NewReconciler(attribution.ReconcilerConfig{
		Cluster: cfg.ClusterName,
		VPCID:   cfg.VPCID,
	}, kube.NewClient(kubeClient), awsec2.NewClient(ec2.NewFromConfig(awsConfig)), logger)

	app := server.New(server.Config{
		BuildCommit:        commit,
		BuildVersion:       version,
		ClusterName:        cfg.ClusterName,
		ListenAddress:      cfg.ListenAddress,
		ReconcileInterval:  cfg.ReconcileInterval,
		StalenessThreshold: cfg.StalenessThreshold,
	}, reconciler, logger)

	if err := app.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func kubernetesConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return nil, err
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
