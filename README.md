# aws-eks-ipamd-attribution-exporter

Prometheus exporter for attributing AWS VPC private IPv4 usage back to EKS/IPAMD ownership.

It reconciles Kubernetes pods/nodes with AWS ENI private IPs, then exposes low-cardinality metrics that show whether consumed subnet IPs are attached to workload pods, sitting warm on node ENIs, reserved as ENI primary IPs, or consumed by non-Kubernetes infrastructure.

## What It Exposes

- `ipamd_attribution_ips{cluster,subnet_id,subnet_name,az,nodepool,instance_type,state}`
- `ipamd_attribution_reconcile_duration_seconds`
- `ipamd_attribution_reconcile_errors_total{reason}`
- `ipamd_attribution_pod_ip_mismatches{reason}`
- `ipamd_attribution_last_success_timestamp_seconds`
- `ipamd_attribution_build_info`

States:

- `workload_assigned`: secondary node ENI IPv4 matches an active non-hostNetwork pod IP.
- `warm_unassigned`: secondary node ENI IPv4 is allocated by IPAMD but not assigned to an active pod.
- `eni_primary_reserved`: node ENI primary IPv4.
- `non_k8s`: ENI is not attached to a known Kubernetes node.
- `unknown`: AWS-observed IP cannot be safely classified.

## Endpoints

- `/metrics`: Prometheus metrics served from the last reconcile cache.
- `/healthz`: readiness endpoint; returns 200 only after a successful reconcile and while it is fresh.
- `/inventory.json`: exact IP-level drilldown rows for incident debugging.

The exporter never calls AWS or Kubernetes from the `/metrics` handler.

## Configuration

Required environment variables:

- `CLUSTER_NAME`: stable cluster label value.
- `VPC_ID`: VPC to inspect.
- `AWS_REGION`: AWS region for EC2 API calls.

Optional environment variables:

- `LISTEN_ADDRESS`: default `:8080`.
- `RECONCILE_INTERVAL`: default `60s`.
- `STALE_THRESHOLD`: default `180s`.

## AWS Permissions

The IRSA role needs read-only EC2 permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeSubnets"
      ],
      "Resource": "*"
    }
  ]
}
```

`ec2:DescribeInstances` is intentionally not required for v1. Node attribution uses Kubernetes node `spec.providerID` and ENI attachment instance IDs.

## Kubernetes Permissions

The exporter needs cluster-wide read access to pods and nodes:

- `get`, `list`, `watch` on `pods`
- `get`, `list`, `watch` on `nodes`

## Build

Requires Go 1.26 or newer.

```sh
go test ./...
docker build -t ipamd-attribution-exporter:dev .
```

## Example Deployment

See [examples/kubernetes/ipamd-attribution-exporter.yaml](examples/kubernetes/ipamd-attribution-exporter.yaml).

The example is intended as copy-pasteable GitOps input. Replace the ECR image, IRSA role ARN, cluster name, and VPC ID.
