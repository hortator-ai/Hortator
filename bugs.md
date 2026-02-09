# Hortator Bugs / Issues Found During Install

_Discovered during sandbox deployment on 2026-02-09_

---

## BUG-002: ✅ FIXED — Helm chart CRDs vs standalone CRDs
- **Fix:** Removed template CRDs, using Helm `crds/` directory convention now. Standalone `crds/` kept for kubectl-only installs.

## BUG-003: ✅ FIXED — Example YAML uses wrong apiVersion
- **Context:** `examples/advanced/multi-tier.yaml` uses `hortator.io/v1alpha1` but CRDs are registered as `core.hortator.ai/v1alpha1`
- **Suggestion:** Fix all examples to use `core.hortator.ai/v1alpha1`

## BUG-004: ✅ FIXED — Example ClusterAgentRole fields don't match CRD schema
- **Context:** Example uses `spec.tier`, `spec.capabilities`, `spec.model`, `spec.budget`, `spec.timeout` on ClusterAgentRole, but the CRD only has `spec.description`, `spec.defaultModel`, `spec.rules`, `spec.tools`, `spec.references`, `spec.antiPatterns`, `spec.health`
- **Suggestion:** Either update the CRD to include these fields, or rewrite the examples to match the actual schema

## BUG-005: ✅ FIXED — Example AgentTask uses `roleRef` / `retainPVC` — not in CRD
- **Context:** Example uses `spec.roleRef` and `spec.retainPVC` but CRD has `spec.role` and no `retainPVC`
- **Suggestion:** Fix examples to use `spec.role` and either add `retainPVC` to CRD or remove from example

## BUG-006: Operator RBAC missing ConfigMap/Lease permissions for leader election
- **Context:** Operator SA `hortator` can't list ConfigMaps at cluster scope. controller-runtime needs ConfigMaps (or Leases) for leader election + the operator likely needs them to read Helm config. Operator logs spam `configmaps is forbidden` and never reconciles tasks.
- **Suggestion:** Add ConfigMaps + Leases + Secrets to the ClusterRole (or namespaced Role in hortator-system). Also needs Pods, PVCs, Secrets in target namespaces to actually spawn agent pods.

## BUG-007: ✅ FIXED — Controller doesn't inject API key from model.apiKeyRef into agent pod env
- **Context:** Task spec has `model.apiKeyRef.secretName` and `model.apiKeyRef.key`, but the controller doesn't create an env var from the secret reference. The entrypoint needs `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` but neither is set.
- **Suggestion:** Controller should map `apiKeyRef` to `ANTHROPIC_API_KEY` (or a generic `LLM_API_KEY`) env var using `valueFrom.secretKeyRef`

## BUG-008: ✅ FIXED — Agent image hardcoded as `:latest` in controller (3 places)
- **Context:** Lines 234, 245, 787 of `agenttask_controller.go` hardcode `ghcr.io/hortator-ai/agent:latest`. ConfigMap override works but shouldn't need it.
- **Suggestion:** Read from Helm-injected env var or config, don't hardcode `:latest`

## BUG-009: ✅ FIXED — No StorageClass shipped or documented for RKE2
- **Context:** RKE2 has no default StorageClass. PVCs hang in Pending until local-path-provisioner is installed.
- **Suggestion:** Document requirement or optionally deploy local-path-provisioner via Helm dependency

## BUG-010: ✅ FIXED — CRD schema / Go types mismatch on SecretKeyRef
- **Context:** Go struct has `SecretName string \`json:"secretName"\`` but the CRD openAPI schema generated uses `name` as the field. When YAML uses `name:` (to pass CRD validation), the Go deserializer maps it to nothing because the json tag is `secretName`. When YAML uses `secretName:`, CRD strict validation rejects it as unknown field.
- **Root cause:** CRD YAML in `crds/agenttask.yaml` was manually written/edited and doesn't match the Go types. Or `controller-gen` wasn't re-run after changing the struct.
- **Fix:** Re-run `controller-gen` to regenerate CRDs from Go types, OR manually fix the CRD YAML to use `secretName` instead of `name`

## BUG-011: Agent entrypoint crashes due to presidio sidecar race condition
- **Context:** `set -euo pipefail` + `presidio_scan()` calls `curl -s $PRESIDIO_ENDPOINT/analyze` which fails because presidio sidecar isn't ready yet (takes ~5-10s to boot). Curl failure exits the script immediately.
- **Suggestion:** Add retry/wait loop for presidio readiness, or wrap the curl in `|| true`, or use a startup probe

## BUG-012: Entrypoint backgrounds curl inside command substitution — RESPONSE never set
- **Context:** `RESPONSE=$(curl ...) &` backgrounds the subshell, so the parent shell never receives the value. `set -u` then crashes on `$RESPONSE` being unbound.
- **Fix:** Don't background the curl. Use `curl ... > /tmp/response.json` + `RESPONSE=$(cat /tmp/response.json)`, or just run curl synchronously (SIGTERM trap still works).

## BUG-013: ✅ FIXED — Presidio sidecar OOMKilled (512Mi limit)
- **Fix:** Bumped presidio memory to 512Mi request / 1Gi limit in Helm values.
- **Remaining:** Controller should check agent container exit code, not pod.phase (sidecar failure shouldn't fail the task). Tracked separately.

---

_Add more as we go._
