package kube

import (
	"context"
	"net/netip"

	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/attribution"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Client struct {
	client kubernetes.Interface
}

func NewClient(client kubernetes.Interface) *Client {
	return &Client{client: client}
}

func (c *Client) ListPods(ctx context.Context) ([]attribution.Pod, error) {
	list, err := c.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	pods := make([]attribution.Pod, 0, len(list.Items))
	for _, pod := range list.Items {
		pods = append(pods, attribution.Pod{
			Namespace:   pod.Namespace,
			Name:        pod.Name,
			Phase:       string(pod.Status.Phase),
			HostNetwork: pod.Spec.HostNetwork,
			PodIPs:      podIPs(pod),
			OwnerKind:   ownerKind(pod),
			OwnerName:   ownerName(pod),
		})
	}
	return pods, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]attribution.Node, error) {
	list, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	nodes := make([]attribution.Node, 0, len(list.Items))
	for _, node := range list.Items {
		nodes = append(nodes, attribution.Node{
			Name:       node.Name,
			ProviderID: node.Spec.ProviderID,
			Labels:     node.Labels,
		})
	}
	return nodes, nil
}

func podIPs(pod corev1.Pod) []netip.Addr {
	seen := map[netip.Addr]struct{}{}
	var out []netip.Addr
	add := func(value string) {
		ip, err := netip.ParseAddr(value)
		if err != nil {
			return
		}
		if _, ok := seen[ip]; ok {
			return
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}

	add(pod.Status.PodIP)
	for _, podIP := range pod.Status.PodIPs {
		add(podIP.IP)
	}
	return out
}

func ownerKind(pod corev1.Pod) string {
	if len(pod.OwnerReferences) == 0 {
		return ""
	}
	return pod.OwnerReferences[0].Kind
}

func ownerName(pod corev1.Pod) string {
	if len(pod.OwnerReferences) == 0 {
		return ""
	}
	return pod.OwnerReferences[0].Name
}
