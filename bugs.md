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

## BUG-006: ✅ FIXED — Operator RBAC missing ConfigMap/Lease permissions for leader election
- **Fix:** Helm chart RBAC was already comprehensive. Kustomize path `config/rbac/role.yaml` updated to add Secrets (get/list/watch), pods/exec (create), and AgentRole/ClusterAgentRole (get/list/watch). Leader election role binding namespace fixed from `system` to `hortator-system`. (2026-02-11)

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

## BUG-014: ✅ FIXED — Presidio native sidecar exits 137 (SIGKILL) — expected but noisy
- **Fix:** Added `preStop` lifecycle hook (`sleep 5`) to the Presidio deployment to allow graceful drain. Documented in Helm NOTES.txt that exit 137 on Presidio during pod transitions is expected. (2026-02-11)

## BUG-015: ✅ FIXED — Presidio not reachable — "WARN: Presidio not reachable, skipping PII scan"
- **Context:** Both hello-world and build-rest-api log `WARN: Presidio not reachable, skipping PII scan`. The Presidio service isn't ready when the agent entrypoint starts polling.
- **Severity:** Medium. PII scanning is silently skipped on every task.
- **Root cause:** Presidio Deployment (centralized service) takes 30-60s to start on first boot (spaCy model loading). The original 30s wait loop was too short.
- **Fix:** Increased default Presidio wait timeout from 30s to 60s in both runtimes (`entrypoint.sh` and `main.py`). Made configurable via `PRESIDIO_WAIT_SECONDS` env var for clusters with slow starts. (2026-02-11)

## BUG-016: ✅ FIXED — Reconciler conflict error on status update
- **Fix:** Added `updateStatusWithRetry()` helper using `retry.RetryOnConflict` from `k8s.io/client-go/util/retry`. Re-fetches the latest CR version before each retry attempt. All ~22 `Status().Update()` calls in `agenttask_controller.go` and `helpers.go` now use the retry wrapper. (2026-02-11)

## BUG-017: ✅ FIXED — Task ID always "unknown" in runtime logs
- **Fix:** `pod_builder.go` now injects `taskId: task.Name` into the `task.json` payload. Both runtimes (`entrypoint.sh` and `main.py`) also fall back to the `HORTATOR_TASK_NAME` env var when `taskId` is missing from task.json. Belt-and-suspenders approach. (2026-02-11)

## BUG-018: ✅ FIXED — build-rest-api (tribune) didn't actually spawn children
- **Context:** The multi-tier task completed with only 133 input / 4096 output tokens and no child tasks were created. Tribune used bash single-shot runtime instead of the Python agentic runtime.
- **Severity:** High. Multi-tier orchestration (the core value prop) wasn't working end-to-end.
- **Root causes (multiple):**
  1. **CI didn't build/push the `agent-agentic` image.** The `ci.yaml` release job and E2E job only built operator + bash agent images, not the agentic runtime image. Without the image on the registry, tribune pods fell back to the bash runtime (single LLM call, no tools).
  2. **Agent pods missing NetworkPolicy labels.** Pods lacked `hortator.ai/managed: "true"` and `hortator.ai/cap-*` labels, so capability-based NetworkPolicies (spawn egress to K8s API, Presidio access) never matched.
  3. **`LITELLM_API_BASE` wrongly set for known providers.** The agentic runtime set `LITELLM_API_BASE` to `https://api.anthropic.com/v1` which broke litellm's Anthropic routing (it tried OpenAI-compatible `/chat/completions` path against Anthropic's `/messages` endpoint).
  4. **Tribune/centurion didn't auto-include `spawn` capability.** If user omitted `spawn` from capabilities, the agentic runtime got no spawn tools and the LLM answered directly instead of delegating.
  5. **Quickstart script didn't build agentic image or pass correct Helm values.** Used wrong `--set` paths and skipped the agentic Dockerfile entirely.
- **Fix (2026-02-11):**
  1. Added `agent-agentic` build to `ci.yaml` release job, E2E job, and build job.
  2. Pod builder now sets `hortator.ai/managed: "true"` and per-capability labels (`hortator.ai/cap-spawn`, etc.) on all agent pods.
  3. Agentic runtime only sets `LITELLM_API_BASE` for custom endpoints (not anthropic.com or openai.com).
  4. Tribune/centurion tiers auto-inject `spawn` into effective capabilities (pod labels + env var) even if not specified in the AgentTask spec.
  5. Quickstart script builds all three images and uses correct Helm values paths.

---

_Add more as we go._
