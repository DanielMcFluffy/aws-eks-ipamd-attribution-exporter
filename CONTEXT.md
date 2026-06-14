# Context Glossary

## workload_assigned

An AWS-observed secondary private IPv4 address on a Kubernetes node ENI that matches an active non-hostNetwork pod IP.

## warm_unassigned

An AWS-observed secondary private IPv4 address on a Kubernetes node ENI that is allocated to the node by IPAMD but not currently assigned to an active workload pod.

## eni_primary_reserved

The primary private IPv4 address on a Kubernetes node ENI. It consumes subnet capacity but is not counted as warm pod capacity.

## non_k8s

An AWS-observed private IPv4 address on an ENI that is not attached to a known Kubernetes node.

## unknown

An AWS-observed private IPv4 address that cannot be classified safely because the observed state is ambiguous or contradictory.

## nodepool

A normalized node provisioning pool. Prefer the Karpenter NodePool label, then EKS node group labels, then `unknown`.

