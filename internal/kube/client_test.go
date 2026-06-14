package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListPodsCollectsPodIPAndPodIPsWithoutDuplicates(t *testing.T) {
	client := NewClient(fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "apps",
			Name:      "api",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet",
				Name: "api-abc",
			}},
		},
		Spec: corev1.PodSpec{HostNetwork: false},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.1.10",
			PodIPs: []corev1.PodIP{
				{IP: "10.0.1.10"},
				{IP: "fd00::1"},
			},
		},
	}))

	pods, err := client.ListPods(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 {
		t.Fatalf("pods = %d, want 1", len(pods))
	}
	if len(pods[0].PodIPs) != 2 {
		t.Fatalf("pod IPs = %v, want IPv4 and IPv6 without duplicate IPv4", pods[0].PodIPs)
	}
	if pods[0].OwnerKind != "ReplicaSet" || pods[0].OwnerName != "api-abc" {
		t.Fatalf("owner = %s/%s, want ReplicaSet/api-abc", pods[0].OwnerKind, pods[0].OwnerName)
	}
}
