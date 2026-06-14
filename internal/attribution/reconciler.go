package attribution

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"time"
)

type ReconcilerConfig struct {
	Cluster string
	VPCID   string
}

type Reconciler struct {
	cfg    ReconcilerConfig
	kube   KubernetesReader
	aws    AWSReader
	logger *slog.Logger
}

func NewReconciler(cfg ReconcilerConfig, kube KubernetesReader, aws AWSReader, logger *slog.Logger) *Reconciler {
	return &Reconciler{cfg: cfg, kube: kube, aws: aws, logger: logger}
}

func (r *Reconciler) Reconcile(ctx context.Context) (Result, error) {
	started := time.Now()
	result := Result{
		Metrics:    map[MetricKey]float64{},
		Mismatches: map[string]float64{},
		Errors:     map[string]float64{},
		StartedAt:  started,
	}

	pods, err := r.kube.ListPods(ctx)
	if err != nil {
		result.Errors["kubernetes_pods"]++
		return result.finish(), fmt.Errorf("list pods: %w", err)
	}
	nodes, err := r.kube.ListNodes(ctx)
	if err != nil {
		result.Errors["kubernetes_nodes"]++
		return result.finish(), fmt.Errorf("list nodes: %w", err)
	}
	subnets, err := r.aws.ListSubnets(ctx, r.cfg.VPCID)
	if err != nil {
		result.Errors["aws_subnets"]++
		return result.finish(), fmt.Errorf("list subnets: %w", err)
	}

	subnetIDs := make([]string, 0, len(subnets))
	subnetByID := map[string]Subnet{}
	for _, subnet := range subnets {
		subnetIDs = append(subnetIDs, subnet.ID)
		subnetByID[subnet.ID] = subnet
	}
	sort.Strings(subnetIDs)

	enis, err := r.aws.ListNetworkInterfaces(ctx, subnetIDs)
	if err != nil {
		result.Errors["aws_network_interfaces"]++
		return result.finish(), fmt.Errorf("list network interfaces: %w", err)
	}

	podByIP := activePodIPMap(pods)
	instanceToNode := instanceNodeMap(nodes)
	awsIPOccurrences := countAWSIPs(enis)
	seenPodIPs := map[netip.Addr]struct{}{}

	for _, eni := range enis {
		subnet := subnetByID[eni.SubnetID]
		node, isNodeENI := instanceToNode[eni.AttachmentInstanceID]
		for _, privateIP := range eni.PrivateIPs {
			if !privateIP.Address.Is4() {
				continue
			}

			state := StateNonK8s
			unknownReason := ""
			var pod Pod
			hasPod := false

			if awsIPOccurrences[privateIP.Address] > 1 {
				state = StateUnknown
				unknownReason = "duplicate_aws_ip"
				result.Errors[unknownReason]++
			} else if isNodeENI {
				if privateIP.Primary {
					state = StateENIPrimaryReserved
				} else if matchedPod, ok := podByIP[privateIP.Address]; ok {
					state = StateWorkloadAssigned
					pod = matchedPod
					hasPod = true
					seenPodIPs[privateIP.Address] = struct{}{}
				} else {
					state = StateWarmUnassigned
				}
			}

			key := MetricKey{
				Cluster:      r.cfg.Cluster,
				SubnetID:     valueOrUnknown(subnet.ID),
				SubnetName:   valueOrUnknown(subnet.Name),
				AZ:           valueOrUnknown(subnet.AvailabilityZone),
				Nodepool:     "non_k8s",
				InstanceType: "unknown",
				State:        state,
			}
			row := InventoryRow{
				IP:             privateIP.Address.String(),
				State:          state,
				SubnetID:       valueOrUnknown(subnet.ID),
				SubnetName:     valueOrUnknown(subnet.Name),
				AZ:             valueOrUnknown(subnet.AvailabilityZone),
				ENIID:          eni.ID,
				ENIDescription: eni.Description,
				ENIType:        eni.InterfaceType,
				Nodepool:       "non_k8s",
				InstanceType:   "unknown",
				UnknownReason:  unknownReason,
			}
			if isNodeENI {
				key.Nodepool = normalizedNodepool(node.Labels)
				key.InstanceType = valueOrUnknown(node.Labels["node.kubernetes.io/instance-type"])
				row.Node = node.Name
				row.Nodepool = key.Nodepool
				row.InstanceType = key.InstanceType
			}
			if hasPod {
				row.Namespace = pod.Namespace
				row.PodName = pod.Name
				row.OwnerKind = pod.OwnerKind
				row.OwnerName = pod.OwnerName
			}

			result.Metrics[key]++
			result.Inventory = append(result.Inventory, row)
		}
	}

	for ip := range podByIP {
		if _, ok := seenPodIPs[ip]; !ok {
			result.Mismatches["pod_ip_not_found_on_aws_eni"]++
		}
	}

	sort.Slice(result.Inventory, func(i, j int) bool {
		left := result.Inventory[i]
		right := result.Inventory[j]
		return strings.Join([]string{left.SubnetID, left.ENIID, left.IP}, "/") < strings.Join([]string{right.SubnetID, right.ENIID, right.IP}, "/")
	})

	return result.finish(), nil
}

func (r Result) finish() Result {
	r.FinishedAt = time.Now()
	r.Duration = r.FinishedAt.Sub(r.StartedAt)
	return r
}

func activePodIPMap(pods []Pod) map[netip.Addr]Pod {
	out := map[netip.Addr]Pod{}
	for _, pod := range pods {
		if pod.HostNetwork || pod.Phase == "Succeeded" || pod.Phase == "Failed" {
			continue
		}
		for _, ip := range pod.PodIPs {
			if ip.Is4() {
				out[ip] = pod
			}
		}
	}
	return out
}

func instanceNodeMap(nodes []Node) map[string]Node {
	out := map[string]Node{}
	for _, node := range nodes {
		instanceID := instanceIDFromProviderID(node.ProviderID)
		if instanceID == "" {
			continue
		}
		out[instanceID] = node
	}
	return out
}

func instanceIDFromProviderID(providerID string) string {
	if providerID == "" {
		return ""
	}
	parts := strings.Split(providerID, "/")
	return parts[len(parts)-1]
}

func countAWSIPs(enis []NetworkInterface) map[netip.Addr]int {
	out := map[netip.Addr]int{}
	for _, eni := range enis {
		for _, privateIP := range eni.PrivateIPs {
			if privateIP.Address.Is4() {
				out[privateIP.Address]++
			}
		}
	}
	return out
}

func normalizedNodepool(labels map[string]string) string {
	for _, key := range []string{
		"karpenter.sh/nodepool",
		"eks.amazonaws.com/nodegroup",
		"alpha.eksctl.io/nodegroup-name",
	} {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return "unknown"
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
