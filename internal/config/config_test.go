package config

import (
	"testing"
	"time"
)

func TestFromEnvRequiresCoreScope(t *testing.T) {
	t.Setenv("CLUSTER_NAME", "")
	t.Setenv("VPC_ID", "")
	t.Setenv("AWS_REGION", "")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected missing required env error")
	}
}

func TestFromEnvParsesDurations(t *testing.T) {
	t.Setenv("CLUSTER_NAME", "production")
	t.Setenv("VPC_ID", "vpc-123")
	t.Setenv("AWS_REGION", "ap-southeast-1")
	t.Setenv("RECONCILE_INTERVAL", "30s")
	t.Setenv("STALE_THRESHOLD", "2m")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReconcileInterval != 30*time.Second {
		t.Fatalf("reconcile interval = %s, want 30s", cfg.ReconcileInterval)
	}
	if cfg.StalenessThreshold != 2*time.Minute {
		t.Fatalf("staleness threshold = %s, want 2m", cfg.StalenessThreshold)
	}
}

func TestFromEnvRejectsInvalidDuration(t *testing.T) {
	t.Setenv("CLUSTER_NAME", "production")
	t.Setenv("VPC_ID", "vpc-123")
	t.Setenv("AWS_REGION", "ap-southeast-1")
	t.Setenv("RECONCILE_INTERVAL", "soon")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
}
