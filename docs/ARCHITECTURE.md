# Kube Blackbox — Architecture v0

## Mission

Kube Blackbox is an always-on, deterministic Kubernetes incident flight recorder. It reconstructs **what changed, in what order, and what objects were related** before and during an incident.

It is not an AI agent, not an autonomous remediation system, and not a replacement for Prometheus/Loki.

## Non-negotiable properties

1. **No Internet dependency at runtime.** The core talks only to the Kubernetes API and local/in-cluster components explicitly enabled by the operator.
2. **No LLM/AI dependency.** All recording, correlation, rules, timelines and reports are deterministic.
3. **Read-only Kubernetes access.** Core MVP requires `get/list/watch`; it never changes workloads.
4. **Embedded/local persistence.** No PostgreSQL, Kafka, Elasticsearch, Prometheus or SaaS is required.
5. **Sensitive-data minimization.** Secrets are never stored. Plaintext environment values and risky annotations are redacted by default.
6. **Bounded storage.** Rolling retention by time and bytes is mandatory.
7. **Portable evidence.** An incident can be exported into a versioned `.kbb` bundle for offline analysis.

## Components

```text
                Kubernetes API
                      |
                list / watch
                      |
              +-------v--------+
              | recorder       |
              | collector      |
              +-------+--------+
                      |
                normalize/redact
                      |
              +-------v--------+
              | record bus     |
              +---+---------+--+
                  |         |
                  |         +-------------------+
                  v                             v
          +-------+--------+            +-------+--------+
          | local store    |            | rule engine    |
          | rolling WAL    |            | deterministic  |
          +-------+--------+            +-------+--------+
                  |                             |
                  +-------------+---------------+
                                v
                         +------+-------+
                         | timeline /   |
                         | correlation  |
                         +------+-------+
                                |
                    +-----------+-----------+
                    |                       |
                    v                       v
               CLI / local API        .kbb export
                                            |
                                     future adapters
                                     (out of core)
```

## MVP data sources

### Kubernetes API

- Pod
- Deployment
- ReplicaSet
- StatefulSet
- DaemonSet
- Service
- ConfigMap (payload redacted by default)
- EndpointSlice
- NetworkPolicy
- Node
- PersistentVolumeClaim
- PersistentVolume
- Event

Later: Job/CronJob, ReplicaSet, HPA, PDB, Gateway API, Ingress, VolumeAttachment, ResourceClaim/ResourceSlice and selected CRDs.

### Node Agent — phase 2

A DaemonSet will emit **signals**, not arbitrary host dumps:

- kubelet/containerd restart
- OOM kill
- PSI pressure
- conntrack pressure
- interface up/down
- route changes
- DNS/packet failure counters where available
- CNI-specific adapter signals

The node agent is optional. The central recorder remains useful without it.

## Canonical record

Every source is converted to the same immutable record schema:

```json
{
  "schemaVersion": "kbb.io/record/v1alpha1",
  "id": "...",
  "observedAt": "2026-09-08T09:00:00Z",
  "source": "kubernetes-api",
  "action": "UPDATE",
  "cluster": "prod-a",
  "apiVersion": "v1",
  "kind": "Pod",
  "namespace": "payment",
  "name": "payment-abc",
  "uid": "...",
  "resourceVersion": "12345",
  "object": {},
  "previous": {}
}
```

This is the key architectural boundary. The storage, timeline engine and future analyzers do not need to know how the event was collected.

## Deterministic correlation

Correlation must be evidence-based. Initial graph edges:

- Deployment -> ReplicaSet -> Pod through ownerReferences
- Service -> EndpointSlice -> Pod through labels/targetRef
- Pod -> Node through spec.nodeName
- Pod -> PVC -> PV
- Pod -> ConfigMap/Secret **reference metadata only**
- NetworkPolicy -> selected Pods
- Event -> involvedObject UID

Example:

```text
Deployment/payment
  -> ReplicaSet/payment-76d
     -> Pod/payment-76d-x1
        -> Node/worker-04

Service/payment
  -> EndpointSlice/payment-abc
     -> Pod/payment-76d-x1
```

Rules can then produce statements like `all ready endpoints disappeared after rollout` without an LLM.

## Storage strategy

### v0

Segmented JSONL append-only store:

- simple;
- inspectable with standard tools;
- no runtime dependency;
- easy export and repair;
- rolling time/size retention.

### v1

Replace behind `Store` interface with an indexed embedded engine after real load tests. Candidate selection must be benchmark-driven; the public record schema must not change because of the database choice.

## No-AI boundary and future extension

The core will never import an LLM SDK.

Future integration is only through a versioned export/analysis contract:

```text
core -> incident.kbb -> external analyzer -> findings.json
```

or a local process protocol:

```text
stdin:  AnalysisRequest v1
stdout: AnalysisResult v1
```

An analyzer may be rule-based, an offline local model, a corporate LLM gateway or nothing at all. Kube Blackbox works identically when no analyzer exists.

## Security model

- no Secret payload collection;
- ConfigMap payload collection disabled by default;
- env `.value` redacted by default;
- no `pods/log`, `pods/exec` or mutation permissions in core MVP;
- recorder container runs non-root with dropped capabilities;
- data directory should reside on an encrypted volume if incident data is sensitive;
- future export supports checksums/signatures, but signing is not required for MVP.
