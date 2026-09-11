# MVP roadmap

## M0 — foundation (this scaffold)

- [x] Canonical immutable record format
- [x] Kubernetes read-only watcher
- [x] Local segmented JSONL persistence
- [x] Time/size rolling retention
- [x] Default redaction policy
- [x] Basic timeline query
- [x] Restricted RBAC deployment example
- [x] Unit tests
- [x] Real-cluster smoke test (`demo214`, chart `0.1.3`; see `docs/SHTURVAL.md`)

## M1 — useful flight recorder

- [x] Initial informer list marked as `SNAPSHOT`
- [x] Structured object diff (JSON Pointer paths)
- [ ] `kubectl blackbox timeline`
- [ ] `kubectl blackbox diff --from --to`
- [ ] ownerReference graph
- [ ] Service -> EndpointSlice -> Pod graph
- [ ] Event -> object correlation by UID
- [ ] incident trigger rules: CrashLoopBackOff, readiness collapse, Pending timeout, Node NotReady
- [ ] `.kbb` portable incident bundle

## M2 — node signals

- [ ] optional privileged/minimal-capability DaemonSet design
- [ ] kubelet/containerd lifecycle watcher
- [ ] kernel OOM signals
- [ ] PSI pressure
- [ ] conntrack pressure
- [ ] interface/route change signals
- [ ] Cilium/Calico adapters without hard dependency

## M3 — deterministic RCA

- [ ] graph-based causal candidates
- [ ] rule confidence/evidence model
- [ ] `blackbox why pod/...`
- [ ] `blackbox why service/...`
- [ ] incident report with evidence IDs
- [ ] no generative text required

## Definition of MVP success

Given a test cluster where a Deployment rollout changes a NetworkPolicy and causes readiness failures, Kube Blackbox must reconstruct the ordered timeline **after the affected Pod has already been deleted**, using only its local retained records.

The standalone `kube-blackbox timeline` and `kube-blackbox diff` commands are
implemented and verified against the live PVC. Packaging them as a `kubectl`
plugin and executing the complete NetworkPolicy/readiness acceptance scenario
remain open M1 work.
