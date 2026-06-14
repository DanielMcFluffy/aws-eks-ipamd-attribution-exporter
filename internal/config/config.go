package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddress      = ":8080"
	defaultReconcileInterval  = 60 * time.Second
	defaultStalenessThreshold = 180 * time.Second
)

type Config struct {
	ClusterName        string
	VPCID              string
	AWSRegion          string
	ListenAddress      string
	ReconcileInterval  time.Duration
	StalenessThreshold time.Duration
}

func FromEnv() (Config, error) {
	reconcileInterval, err := durationEnv("RECONCILE_INTERVAL", defaultReconcileInterval)
	if err != nil {
		return Config{}, err
	}
	stalenessThreshold, err := durationEnv("STALE_THRESHOLD", defaultStalenessThreshold)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ClusterName:        strings.TrimSpace(os.Getenv("CLUSTER_NAME")),
		VPCID:              strings.TrimSpace(os.Getenv("VPC_ID")),
		AWSRegion:          strings.TrimSpace(os.Getenv("AWS_REGION")),
		ListenAddress:      envDefault("LISTEN_ADDRESS", defaultListenAddress),
		ReconcileInterval:  reconcileInterval,
		StalenessThreshold: stalenessThreshold,
	}

	var missing []string
	if cfg.ClusterName == "" {
		missing = append(missing, "CLUSTER_NAME")
	}
	if cfg.VPCID == "" {
		missing = append(missing, "VPC_ID")
	}
	if cfg.AWSRegion == "" {
		missing = append(missing, "AWS_REGION")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	if cfg.ReconcileInterval <= 0 {
		return Config{}, errors.New("RECONCILE_INTERVAL must be positive")
	}
	if cfg.StalenessThreshold <= 0 {
		return Config{}, errors.New("STALE_THRESHOLD must be positive")
	}
	if host, port, err := net.SplitHostPort(cfg.ListenAddress); err != nil || port == "" {
		if strings.HasPrefix(cfg.ListenAddress, ":") {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("LISTEN_ADDRESS must be host:port or :port, got %q (host %q)", cfg.ListenAddress, host)
	}

	return cfg, nil
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return parsed, nil
}
