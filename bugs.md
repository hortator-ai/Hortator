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

## BUG-014: Presidio native sidecar exits 137 (SIGKILL) — expected but noisy
- **Context:** After converting Presidio from regular container to native sidecar (init container with `restartPolicy=Always`), K8s terminates it with SIGKILL (exit 137) when the agent completes. This is correct behavior — native sidecars get killed after main containers exit. But the exit code 137 / reason "Error" looks alarming in `kubectl describe pod`.
- **Severity:** Cosmetic / low. Tasks complete successfully.
- **Suggestion:** Consider adding a `preStop` hook to Presidio that gracefully shuts down, or just document that exit 137 on the sidecar is expected.

## BUG-015: Presidio not reachable — "WARN: Presidio not reachable, skipping PII scan"
- **Context:** Both hello-world and build-rest-api log `WARN: Presidio not reachable, skipping PII scan`. The Presidio sidecar is present (exit 137 confirms it ran), but the agent's entrypoint can't reach it in time. The native sidecar starts as an init container and should be ready before the agent container, but the readiness probe may not gate the agent container start.
- **Severity:** Medium. PII scanning is silently skipped on every task.
- **Root cause:** Native sidecar init containers with `restartPolicy=Always` run alongside the main container, but K8s doesn't wait for their readiness probe before starting the main container. The old `|| return 0` fallback from BUG-011 means the agent just skips the scan.
- **Suggestion:** Add a startup wait loop in the agent entrypoint that polls Presidio's `/health` endpoint (e.g., 30 retries × 1s) before proceeding with the scan. Or use a `postStart` lifecycle hook on the agent container.

## BUG-016: Reconciler conflict error on status update
- **Context:** `Operation cannot be fulfilled on agenttasks.core.hortator.ai "build-rest-api": the object has been modified; please apply your changes to the latest version and try again`
- **Severity:** Low. K8s optimistic concurrency working as designed — the controller retries on next reconcile and succeeds. But it indicates the controller may be doing multiple status updates without re-fetching the latest version.
- **Suggestion:** Add a `retry.RetryOnConflict` wrapper around status updates, or re-fetch the task before each status update in the reconcile loop.

## BUG-017: Task ID always "unknown" in runtime logs
- **Context:** Both tasks log `Task=unknown` — the runtime isn't reading the task ID from `/inbox/task.json` or environment.
- **Severity:** Low. Cosmetic but makes debugging multi-task scenarios harder.
- **Suggestion:** Ensure the operator sets `HORTATOR_TASK_ID` env var or the runtime reads `taskId` from `/inbox/task.json`.

## BUG-018: build-rest-api (tribune) didn't actually spawn children
- **Context:** The multi-tier task completed with only 133 input / 4096 output tokens and no child tasks were created. A tribune task that's supposed to decompose work and delegate to centurions/legionaries should spawn child AgentTasks. It appears the agent just generated a text response without using the `hortator spawn` CLI.
- **Severity:** High. Multi-tier orchestration (the core value prop) isn't working end-to-end.
- **Root cause:** Likely the agent runtime doesn't have the hortator CLI installed, or the prompt/system message doesn't instruct the agent to use it, or the agent's capabilities don't include the right tool bindings.
- **Suggestion:** Verify `hortator` CLI is in the agent image PATH, verify the runtime injects instructions about available tools, and check that `spawn` capability maps to actual CLI access.

---

_Add more as we go._
