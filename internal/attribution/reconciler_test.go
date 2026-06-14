package attribution

import (
	"context"
	"log/slog"
	"net/netip"
	"testing"
)

func TestReconcileClassifiesIPStates(t *testing.T) {
	ctx := context.Background()
	reconciler := NewReconciler(ReconcilerConfig{
		Cluster: "prod",
		VPCID:   "vpc-123",
	}, fakeKube{
		pods: []Pod{{
			Namespace: "orders",
			Name:      "api-123",
			Phase:     "Running",
			PodIPs:    addrs("10.0.1.11"),
			OwnerKind: "ReplicaSet",
			OwnerName: "api-abc",
		}},
		nodes: []Node{{
			Name:       "ip-10-0-1-10",
			ProviderID: "aws:///ap-southeast-1a/i-node1",
			Labels: map[string]string{
				"karpenter.sh/nodepool":            "general",
				"node.kubernetes.io/instance-type": "m6i.large",
			},
		}},
	}, fakeAWS{
		subnets: []Subnet{{
			ID:               "subnet-1",
			Name:             "private-a",
			AvailabilityZone: "ap-southeast-1a",
		}},
		enis: []NetworkInterface{{
			ID:                   "eni-node",
			SubnetID:             "subnet-1",
			AttachmentInstanceID: "i-node1",
			PrivateIPs: []PrivateIP{
				{Address: addr("10.0.1.10"), Primary: true},
				{Address: addr("10.0.1.11")},
				{Address: addr("10.0.1.12")},
			},
		}, {
			ID:            "eni-elb",
			SubnetID:      "subnet-1",
			InterfaceType: "interface",
			PrivateIPs: []PrivateIP{
				{Address: addr("10.0.1.20"), Primary: true},
			},
		}},
	}, slog.Default())

	result, err := reconciler.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}

	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "general", "m6i.large", StateENIPrimaryReserved, 1)
	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "general", "m6i.large", StateWorkloadAssigned, 1)
	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "general", "m6i.large", StateWarmUnassigned, 1)
	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "non_k8s", "unknown", StateNonK8s, 1)

	if len(result.Inventory) != 4 {
		t.Fatalf("inventory rows = %d, want 4", len(result.Inventory))
	}
	if result.Mismatches["pod_ip_not_found_on_aws_eni"] != 0 {
		t.Fatalf("unexpected pod mismatch: %v", result.Mismatches)
	}
}

func TestReconcileReportsPodIPMismatchSeparately(t *testing.T) {
	reconciler := NewReconciler(ReconcilerConfig{Cluster: "prod", VPCID: "vpc-123"}, fakeKube{
		pods: []Pod{{
			Namespace: "orders",
			Name:      "api-123",
			Phase:     "Running",
			PodIPs:    addrs("10.0.1.99"),
		}},
		nodes: []Node{{
			Name:       "node",
			ProviderID: "aws:///ap-southeast-1a/i-node1",
		}},
	}, fakeAWS{
		subnets: []Subnet{{ID: "subnet-1", Name: "private-a", AvailabilityZone: "ap-southeast-1a"}},
		enis: []NetworkInterface{{
			ID:                   "eni-node",
			SubnetID:             "subnet-1",
			AttachmentInstanceID: "i-node1",
			PrivateIPs:           []PrivateIP{{Address: addr("10.0.1.10"), Primary: true}},
		}},
	}, slog.Default())

	result, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got := result.Mismatches["pod_ip_not_found_on_aws_eni"]; got != 1 {
		t.Fatalf("mismatch count = %v, want 1", got)
	}
	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "unknown", "unknown", StateENIPrimaryReserved, 1)
}

func TestReconcileMarksDuplicateAWSIPUnknown(t *testing.T) {
	reconciler := NewReconciler(ReconcilerConfig{Cluster: "prod", VPCID: "vpc-123"}, fakeKube{
		nodes: []Node{{Name: "node", ProviderID: "aws:///ap-southeast-1a/i-node1"}},
	}, fakeAWS{
		subnets: []Subnet{{ID: "subnet-1", Name: "private-a", AvailabilityZone: "ap-southeast-1a"}},
		enis: []NetworkInterface{{
			ID:                   "eni-1",
			SubnetID:             "subnet-1",
			AttachmentInstanceID: "i-node1",
			PrivateIPs:           []PrivateIP{{Address: addr("10.0.1.10"), Primary: true}},
		}, {
			ID:                   "eni-2",
			SubnetID:             "subnet-1",
			AttachmentInstanceID: "i-node1",
			PrivateIPs:           []PrivateIP{{Address: addr("10.0.1.10")}},
		}},
	}, slog.Default())

	result, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "unknown", "unknown", StateUnknown, 2)
	if got := result.Errors["duplicate_aws_ip"]; got != 2 {
		t.Fatalf("duplicate error count = %v, want 2", got)
	}
}

func TestReconcileIgnoresHostNetworkCompletedAndIPv6Pods(t *testing.T) {
	reconciler := NewReconciler(ReconcilerConfig{Cluster: "prod", VPCID: "vpc-123"}, fakeKube{
		pods: []Pod{
			{Name: "host", Namespace: "kube-system", Phase: "Running", HostNetwork: true, PodIPs: addrs("10.0.1.11")},
			{Name: "done", Namespace: "jobs", Phase: "Succeeded", PodIPs: addrs("10.0.1.12")},
			{Name: "dual", Namespace: "apps", Phase: "Running", PodIPs: addrs("fd00::1")},
		},
		nodes: []Node{{Name: "node", ProviderID: "aws:///ap-southeast-1a/i-node1"}},
	}, fakeAWS{
		subnets: []Subnet{{ID: "subnet-1", Name: "private-a", AvailabilityZone: "ap-southeast-1a"}},
		enis: []NetworkInterface{{
			ID:                   "eni-node",
			SubnetID:             "subnet-1",
			AttachmentInstanceID: "i-node1",
			PrivateIPs: []PrivateIP{
				{Address: addr("10.0.1.10"), Primary: true},
				{Address: addr("10.0.1.11")},
				{Address: addr("10.0.1.12")},
			},
		}},
	}, slog.Default())

	result, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	assertMetric(t, result, "prod", "subnet-1", "private-a", "ap-southeast-1a", "unknown", "unknown", StateWarmUnassigned, 2)
	if result.Mismatches["pod_ip_not_found_on_aws_eni"] != 0 {
		t.Fatalf("unexpected mismatch for ignored pods: %v", result.Mismatches)
	}
}

type fakeKube struct {
	pods  []Pod
	nodes []Node
}

func (f fakeKube) ListPods(context.Context) ([]Pod, error) {
	return f.pods, nil
}

func (f fakeKube) ListNodes(context.Context) ([]Node, error) {
	return f.nodes, nil
}

type fakeAWS struct {
	subnets []Subnet
	enis    []NetworkInterface
}

func (f fakeAWS) ListSubnets(context.Context, string) ([]Subnet, error) {
	return f.subnets, nil
}

func (f fakeAWS) ListNetworkInterfaces(context.Context, []string) ([]NetworkInterface, error) {
	return f.enis, nil
}

func assertMetric(t *testing.T, result Result, cluster, subnetID, subnetName, az, nodepool, instanceType, state string, want float64) {
	t.Helper()
	key := MetricKey{
		Cluster:      cluster,
		SubnetID:     subnetID,
		SubnetName:   subnetName,
		AZ:           az,
		Nodepool:     nodepool,
		InstanceType: instanceType,
		State:        state,
	}
	if got := result.Metrics[key]; got != want {
		t.Fatalf("metric %+v = %v, want %v; all metrics: %+v", key, got, want, result.Metrics)
	}
}

func addr(value string) netip.Addr {
	ip, err := netip.ParseAddr(value)
	if err != nil {
		panic(err)
	}
	return ip
}

func addrs(values ...string) []netip.Addr {
	out := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		out = append(out, addr(value))
	}
	return out
}
