# Enrich with Kubernetes

For ovn-kubernetes deployments, the Kubernetes enrichment provider resolves OVN
entities to Kubernetes resources (namespaces, pods and nodes) using a kubeconfig.

Like all enrichment, it is optional and additive.

## Configure with flags

Enable it for the default cluster with `--kube-enrichment`. The standard
`KUBECONFIG` environment variable is read as the default kubeconfig path:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --kube-enrichment \
  --kubeconfig /home/me/.kube/config \
  --kube-context my-cluster
```

| Flag | Env var | Default |
|---|---|---|
| `--kube-enrichment` | `NORTHWATCH_KUBE_ENRICHMENT` | `false` |
| `--kubeconfig` | `KUBECONFIG` | (none) |
| `--kube-context` | `NORTHWATCH_KUBE_CONTEXT` | (none) |

If `--kubeconfig` is empty, the usual in-cluster / default-kubeconfig resolution
applies.

## Configure per cluster

In a multi-cluster config file, use a `kubernetes` enrichment block:

```json
{
  "name": "ovn-k8s",
  "ovn_nb_addr": "tcp:10.0.0.1:6641",
  "ovn_sb_addr": "tcp:10.0.0.1:6642",
  "enrichment": {
    "type": "kubernetes",
    "kubeconfig": "/home/me/.kube/config",
    "kube_context": "my-cluster"
  }
}
```

## What gets resolved

Enrichment keys off the `k8s.ovn.org/*` `external_ids` ovn-kubernetes writes onto
the NB objects. The pod reference also drives an API lookup for the node and
labels:

| OVN entity | `external_ids` / key | Resolved to |
|---|---|---|
| `Logical_Switch_Port` | `k8s.ovn.org/pod` (`namespace/name`), `k8s.ovn.org/nad` | Pod name and namespace; node name and pod labels via an API lookup; the NAD as `extra` |
| `Logical_Switch` | `k8s.ovn.org/network` | Network name |
| `Logical_Router` | `k8s.ovn.org/network` | Network name |
| `NAT` | — | Not resolved |

## Confirm it is on

As with any provider, an active Kubernetes enricher adds the `enrich` capability:

```bash
curl -s http://localhost:8080/api/v1/capabilities
```

A cluster cannot use both OpenStack and Kubernetes enrichment at the same time.
Configure exactly one provider per cluster.

For the provider model and caching, see [Enrichment](/explanation/enrichment).
