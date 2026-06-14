package attribution

import (
	"context"
	"net/netip"
	"time"
)

const (
	StateWorkloadAssigned   = "workload_assigned"
	StateWarmUnassigned     = "warm_unassigned"
	StateENIPrimaryReserved = "eni_primary_reserved"
	StateNonK8s             = "non_k8s"
	StateUnknown            = "unknown"
)

type KubernetesReader interface {
	ListPods(ctx context.Context) ([]Pod, error)
	ListNodes(ctx context.Context) ([]Node, error)
}

type AWSReader interface {
	ListSubnets(ctx context.Context, vpcID string) ([]Subnet, error)
	ListNetworkInterfaces(ctx context.Context, subnetIDs []string) ([]NetworkInterface, error)
}

type Pod struct {
	Namespace   string
	Name        string
	Phase       string
	HostNetwork bool
	PodIPs      []netip.Addr
	OwnerKind   string
	OwnerName   string
}

type Node struct {
	Name       string
	ProviderID string
	Labels     map[string]string
}

type Subnet struct {
	ID               string
	Name             string
	AvailabilityZone string
}

type PrivateIP struct {
	Address netip.Addr
	Primary bool
}

type NetworkInterface struct {
	ID                   string
	Description          string
	InterfaceType        string
	SubnetID             string
	AttachmentInstanceID string
	PrivateIPs           []PrivateIP
}

type MetricKey struct {
	Cluster      string
	SubnetID     string
	SubnetName   string
	AZ           string
	Nodepool     string
	InstanceType string
	State        string
}

type InventoryRow struct {
	IP             string `json:"ip"`
	State          string `json:"state"`
	SubnetID       string `json:"subnet_id"`
	SubnetName     string `json:"subnet_name"`
	AZ             string `json:"az"`
	ENIID          string `json:"eni_id"`
	ENIDescription string `json:"eni_description,omitempty"`
	ENIType        string `json:"eni_type,omitempty"`
	Node           string `json:"node,omitempty"`
	Nodepool       string `json:"nodepool,omitempty"`
	InstanceType   string `json:"instance_type,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	PodName        string `json:"pod_name,omitempty"`
	OwnerKind      string `json:"owner_kind,omitempty"`
	OwnerName      string `json:"owner_name,omitempty"`
	UnknownReason  string `json:"unknown_reason,omitempty"`
}

type Result struct {
	Metrics    map[MetricKey]float64
	Inventory  []InventoryRow
	Mismatches map[string]float64
	Errors     map[string]float64
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
}
