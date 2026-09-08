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
- [ ] Real-cluster smoke test (automated harness is in `hack/kubernetes-smoke-test.sh`; execution requires a cluster)

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
