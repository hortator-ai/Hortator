# Changelog

## [0.6.6](https://github.com/hortator-ai/Hortator/compare/v0.6.5...v0.6.6) (2026-02-17)


### Bug Fixes

* handle reincarnation race with stale pods (BUG-112) ([#100](https://github.com/hortator-ai/Hortator/issues/100)) ([3e2416b](https://github.com/hortator-ai/Hortator/commit/3e2416bd5f0bd4cebd8bdb5adbfd80b5f7a9a01f))

## [0.6.5](https://github.com/hortator-ai/Hortator/compare/v0.6.4...v0.6.5) (2026-02-17)


### Bug Fixes

* delete old pod during reincarnation (BUG-112) ([#98](https://github.com/hortator-ai/Hortator/issues/98)) ([3c84a09](https://github.com/hortator-ai/Hortator/commit/3c84a092e01ac5b21fcef81511fe179b3054cdc3))

## [0.6.4](https://github.com/hortator-ai/Hortator/compare/v0.6.3...v0.6.4) (2026-02-17)


### Bug Fixes

* prevent reincarnation infinite loop (BUG-112) ([#96](https://github.com/hortator-ai/Hortator/issues/96)) ([f6ad161](https://github.com/hortator-ai/Hortator/commit/f6ad161f13c2ed728a8815271178fc823535abd0))

## [0.6.3](https://github.com/hortator-ai/Hortator/compare/v0.6.2...v0.6.3) (2026-02-17)


### Bug Fixes

* add non-root user to runtime images and re-enable runAsNonRoot ([#95](https://github.com/hortator-ai/Hortator/issues/95)) ([27ed1ce](https://github.com/hortator-ai/Hortator/commit/27ed1cee8106b993490bb214c96e45648249bb7c))
* remove runAsNonRoot until runtime images support non-root user ([#93](https://github.com/hortator-ai/Hortator/issues/93)) ([d769768](https://github.com/hortator-ai/Hortator/commit/d76976844a4f3c4a8ed4531df9c5770c239eb128))

## [0.6.2](https://github.com/hortator-ai/Hortator/compare/v0.6.1...v0.6.2) (2026-02-17)


### Bug Fixes

* discover children for reincarnated parent in Waiting phase (BUG-112) ([#91](https://github.com/hortator-ai/Hortator/issues/91)) ([36b2142](https://github.com/hortator-ai/Hortator/commit/36b214231b7cdca2c19ff158f04e8f4238548f28))

## [0.6.1](https://github.com/hortator-ai/Hortator/compare/v0.6.0...v0.6.1) (2026-02-17)


### Bug Fixes

* add fail-closed Presidio mode (BUG-113) ([#88](https://github.com/hortator-ai/Hortator/issues/88)) ([cd192c9](https://github.com/hortator-ai/Hortator/commit/cd192c90b115100ad0b3a077b13cc3c02e0a4154))
* harden agent pod defaults and pin Presidio image (BUG-115, BUG-116) ([#87](https://github.com/hortator-ai/Hortator/issues/87)) ([3118f88](https://github.com/hortator-ai/Hortator/commit/3118f8845645d414320b2fb9df5455732c2f34ee))
* make webhook model validation conditional (BUG-114) ([#89](https://github.com/hortator-ai/Hortator/issues/89)) ([93919f9](https://github.com/hortator-ai/Hortator/commit/93919f9594d8e52dbe4d21fff077c0d0b872a13c))

## [0.6.0](https://github.com/hortator-ai/Hortator/compare/v0.5.7...v0.6.0) (2026-02-16)


### Features

* add goreleaser for cross-platform CLI binaries ([93cb71a](https://github.com/hortator-ai/Hortator/commit/93cb71a5464bfedf3d1ffb7f5dd7647356493f28))


### Bug Fixes

* use dynamic release badge instead of hardcoded version ([ab886db](https://github.com/hortator-ai/Hortator/commit/ab886db080ac77f8b7847e0b60a145de3ea7409c))

## [0.5.7](https://github.com/hortator-ai/Hortator/compare/v0.5.6...v0.5.7) (2026-02-16)


### Bug Fixes

* add v prefix to semver image tags to match Helm chart expectations ([#84](https://github.com/hortator-ai/Hortator/issues/84)) ([0f084da](https://github.com/hortator-ai/Hortator/commit/0f084dabddf42106ad66fc985062393da37254d7))

## [0.5.6](https://github.com/hortator-ai/Hortator/compare/v0.5.5...v0.5.6) (2026-02-16)


### Bug Fixes

* set appVersion to match chart version, fix quickstart secret name ([#82](https://github.com/hortator-ai/Hortator/issues/82)) ([8dfd33b](https://github.com/hortator-ai/Hortator/commit/8dfd33b530a3b148dd9b0e8b8f6cefe9b1b76cda))

## [0.5.5](https://github.com/hortator-ai/Hortator/compare/v0.5.4...v0.5.5) (2026-02-16)


### Bug Fixes

* operator crashloop when webhook.enabled=false (quickstart blocker) ([#80](https://github.com/hortator-ai/Hortator/issues/80)) ([46d4a98](https://github.com/hortator-ai/Hortator/commit/46d4a983962efdb1778241a27637c1a91abc8be8))

## [0.5.4](https://github.com/hortator-ai/Hortator/compare/v0.5.3...v0.5.4) (2026-02-16)


### Bug Fixes

* use static release badge (shields.io can't read private repos) ([968388f](https://github.com/hortator-ai/Hortator/commit/968388f24f2dfe5e9c76d87b23cf82aa15af1366))

## [0.5.3](https://github.com/hortator-ai/Hortator/compare/v0.5.2...v0.5.3) (2026-02-16)


### Bug Fixes

* register children in parent PendingChildren on creation ([#71](https://github.com/hortator-ai/Hortator/issues/71)) ([01fbbe3](https://github.com/hortator-ai/Hortator/commit/01fbbe385f4b14f362a1d52183b42b625eecdca9))
* rewrite delegation test to not spoon-feed decomposition ([#65](https://github.com/hortator-ai/Hortator/issues/65)) ([819111b](https://github.com/hortator-ai/Hortator/commit/819111b3a393d5ba28e63c1a877ec8947f2eb418))
* storage.retain should only preserve PVC, not block task GC ([#64](https://github.com/hortator-ai/Hortator/issues/64)) ([01e982e](https://github.com/hortator-ai/Hortator/commit/01e982e9d2508417ad30cc99d0b9082a084d3f37))
* use TCP probe for LiteLLM instead of HTTP /health ([#66](https://github.com/hortator-ai/Hortator/issues/66)) ([e52f51c](https://github.com/hortator-ai/Hortator/commit/e52f51ca88a669de3976f77742df9c1bb1c0728d))

## [0.5.2](https://github.com/hortator-ai/Hortator/compare/v0.5.1...v0.5.2) (2026-02-16)


### Bug Fixes

* add anti-duplication guidance to tribune delegation prompt ([#60](https://github.com/hortator-ai/Hortator/issues/60)) ([d731ffe](https://github.com/hortator-ai/Hortator/commit/d731ffe1389b66c5d0a2588675a03568f096200f))

## [0.5.1](https://github.com/hortator-ai/Hortator/compare/v0.5.0...v0.5.1) (2026-02-16)


### Bug Fixes

* use Callable type instead of callable builtin in loop.py ([#56](https://github.com/hortator-ai/Hortator/issues/56)) ([c416c88](https://github.com/hortator-ai/Hortator/commit/c416c88a9008a1485c7ebbec901aafa19730a3bb))

## [0.5.0](https://github.com/hortator-ai/Hortator/compare/v0.4.0...v0.5.0) (2026-02-16)


### Features

* add maxIterations and planning loop prompt for iterative agents ([#55](https://github.com/hortator-ai/Hortator/issues/55)) ([479b4b0](https://github.com/hortator-ai/Hortator/commit/479b4b0f18c74da7c986e25dc8abb01a59aa31d5))
* add role metadata fields and context injection for agent awareness ([#50](https://github.com/hortator-ai/Hortator/issues/50)) ([c58dd02](https://github.com/hortator-ai/Hortator/commit/c58dd02366504a760dee2f998c82b8ad385c034f))

## [0.4.0](https://github.com/hortator-ai/Hortator/compare/v0.3.0...v0.4.0) (2026-02-15)


### Features

* add LiteLLM proxy as Helm subchart ([#44](https://github.com/hortator-ai/Hortator/issues/44)) ([ce0d5cf](https://github.com/hortator-ai/Hortator/commit/ce0d5cf54ffada7c61b4a775ebd04de37ab2f994))

## [0.3.0](https://github.com/hortator-ai/Hortator/compare/v0.2.0...v0.3.0) (2026-02-15)


### Features

* add comprehensive AgentTask test manifests ([ea9efab](https://github.com/hortator-ai/Hortator/commit/ea9efabc111982a87cb9a78fbee48a6612a59860))
* add demo manifests without hardcoded namespaces ([8bfaab2](https://github.com/hortator-ai/Hortator/commit/8bfaab20355a4e6edc5412364313e9fc3a5fb360))
* add design documents for retry semantics and file delivery ([6fa03b8](https://github.com/hortator-ai/Hortator/commit/6fa03b88e6e7cf720c375c2504786ea25679514a))
* add hierarchy budget for shared cost control across task trees ([#32](https://github.com/hortator-ai/Hortator/issues/32)) ([db3a677](https://github.com/hortator-ai/Hortator/commit/db3a677f49894979586e161690471bedfe483a07))
* add K8s Events and enriched OTel audit logging ([7f527cc](https://github.com/hortator-ai/Hortator/commit/7f527cc43c1c80a89e1ae6d9f4e7cf12ba165e54))
* add pluggable vector store abstraction with Qdrant implementation ([f55ccf4](https://github.com/hortator-ai/Hortator/commit/f55ccf47362e19e4248793889df6a3fd55c3dfa8))
* add pluggable vector store abstraction with Qdrant implementation ([c8023e7](https://github.com/hortator-ai/Hortator/commit/c8023e7dca481363f3bd6242ba6e7b2d9c55ce34))
* add shell command filtering and read-only workspace to AgentPolicy ([21dc669](https://github.com/hortator-ai/Hortator/commit/21dc669c304d91bdc8b6ca1c356a8937c8915dab))
* add shell command filtering and read-only workspace via AgentPolicy ([8e997ad](https://github.com/hortator-ai/Hortator/commit/8e997ad81c54b66622f10f672a1df16f633f2954))
* add tag-based PVC discovery with prompt keyword matching ([0bbb376](https://github.com/hortator-ai/Hortator/commit/0bbb37668237bcc2fcb12dc3a33d43a271aef652))
* add validating admission webhook for AgentTask CRD ([0c3a3f2](https://github.com/hortator-ai/Hortator/commit/0c3a3f2e5340cc44b3f928197052e5a29c00e9f2))
* agents report results via CRD, every task gets a PVC ([94ee866](https://github.com/hortator-ai/Hortator/commit/94ee866021e03290c5eca984c13d41bae94b45d1))
* complete CLI with delete/version commands and k8s client initialization ([ffa75a8](https://github.com/hortator-ai/Hortator/commit/ffa75a8cdd759d6721a86477a99859279bce07b5))
* enhance CRD management and documentation ([e60edae](https://github.com/hortator-ai/Hortator/commit/e60edae62badd5deaca87edd8c335c319a51cf7d))
* enterprise — AgentPolicy CRD + Presidio sidecar integration ([3d459fe](https://github.com/hortator-ai/Hortator/commit/3d459fe078b237dce288eb59ca169eddc393b9a2))
* extract actual LLM response from runtime output ([1c02f9e](https://github.com/hortator-ai/Hortator/commit/1c02f9e7c59580e0995f5e3121b386bf675b6a57))
* extract token usage from agent logs, record attempts on success ([f906097](https://github.com/hortator-ai/Hortator/commit/f906097374a239e87debcc728e63f320ed1bdad6))
* gateway resolves AgentRole model config for tasks ([fb1b0f2](https://github.com/hortator-ai/Hortator/commit/fb1b0f2427a0b272c8516e297ea066dfcbdb2faa))
* hortator watch TUI with live task tree, details, and logs ([026125d](https://github.com/hortator-ai/Hortator/commit/026125de10b985a6e2b7028cd07567049a021204))
* implement dynamic worker RBAC provisioning ([f56ce2b](https://github.com/hortator-ai/Hortator/commit/f56ce2bad8a8a0d15659876626e170ca56489701))
* implement P0 + P1 MVP ([1070425](https://github.com/hortator-ai/Hortator/commit/1070425e94514bd72ae3b93a7ecb18a8ac988a0d))
* implement real artifact retrieval via ephemeral pod + PVC mount ([422351c](https://github.com/hortator-ai/Hortator/commit/422351c55de7811a6adaded601d00be1bf391970))
* implement retry semantics for transient failures ([d209d13](https://github.com/hortator-ai/Hortator/commit/d209d13433b6eda5597f97ea2e5d75d04cd1c67b))
* **L7:** add typed Go structs for AgentRole and ClusterAgentRole ([8d91dae](https://github.com/hortator-ai/Hortator/commit/8d91daee66f541c1f65beeb0c3538dfaea6e4ba1))
* multi-tenancy namespace isolation model ([2a98e7a](https://github.com/hortator-ai/Hortator/commit/2a98e7a9efc656af757873ed14c1b7e81630c2dd))
* namespace cycling in hortator watch TUI ([c0be5ea](https://github.com/hortator-ai/Hortator/commit/c0be5eac83ff3a3e570ff3641b974c77a83d7783))
* OpenAI-compatible API gateway ([b0d78ab](https://github.com/hortator-ai/Hortator/commit/b0d78abb09a19b286316987710950dadcec67d99))
* P2 implementation — tree, JSON output, OTel, namespace restrictions ([39e273d](https://github.com/hortator-ai/Hortator/commit/39e273d7458ceb04182bc002e587f94575111544))
* per-capability RBAC — split worker ServiceAccounts ([f011b9b](https://github.com/hortator-ai/Hortator/commit/f011b9b2e13f843d2a815896c859a5d05de45866))
* Python SDK with sync/async clients, streaming, LangChain & CrewAI integrations ([965eec8](https://github.com/hortator-ai/Hortator/commit/965eec8f0a5c5e39940850c2a10488bba9944d1e))
* redact PII in prompts and tool results before LLM submission ([#35](https://github.com/hortator-ai/Hortator/issues/35)) ([27dad2d](https://github.com/hortator-ai/Hortator/commit/27dad2d800b6b7bb62162102019b8fb21b9b793d))
* result cache with dedup — content-addressable cache keyed on prompt+role ([ec7a3ad](https://github.com/hortator-ai/Hortator/commit/ec7a3addbcf1722f1552eee6e7d3ce2ff63f9dad))
* scaffold Hortator k8s operator for AI agent orchestration ([e161eec](https://github.com/hortator-ai/Hortator/commit/e161eece6a0bb4bc7ab663edadd3f3a177e5e292))
* split worker ServiceAccount by spawn capability for least-privilege RBAC ([da4f1e8](https://github.com/hortator-ai/Hortator/commit/da4f1e874de9885aae6055f5b42b3ddeb691499d))
* TUI single-pane view for logs, details, describe, summary ([100cc26](https://github.com/hortator-ai/Hortator/commit/100cc26784e94aa4d9f8822babacbab024b17a36))
* TUI single-pane views for logs, details, describe, summary ([2d7ded5](https://github.com/hortator-ai/Hortator/commit/2d7ded59f6b160768bb8fa2dc3813a156d0a3d9c))
* TypeScript SDK with streaming, LangChain.js integration ([98d23d0](https://github.com/hortator-ai/Hortator/commit/98d23d0b37a00d8dc68dad8e4757069dbe74b282))
* update backlog and documentation for recent changes ([9064850](https://github.com/hortator-ai/Hortator/commit/9064850eece34322dbc31a8f44e6e61bf71a69e3))
* validate child task capabilities are subset of parent ([#31](https://github.com/hortator-ai/Hortator/issues/31)) ([78d8e58](https://github.com/hortator-ai/Hortator/commit/78d8e58f23f2ef99bc704807d82d6aea4f304949))
* warm Pod pool for sub-second task assignment ([738a145](https://github.com/hortator-ai/Hortator/commit/738a14526bfde302898f64d0d461f5a142e0b0ab))


### Bug Fixes

* add cleanup steps between test phases to prevent policy pollution ([531684c](https://github.com/hortator-ai/Hortator/commit/531684cb2091134a80a15fbb110bc3190c685c97))
* add per-client rate limiting to gateway ([8417f6e](https://github.com/hortator-ai/Hortator/commit/8417f6e9cce3f65a3a72c876397c6eb2354f5ce2))
* add presidio anonymizer as separate container and endpoint ([2493270](https://github.com/hortator-ai/Hortator/commit/24932702e6a2be1a2cd05282225a8e51a07c26e3))
* align container image names across CI, chart, and Go code ([aad580f](https://github.com/hortator-ai/Hortator/commit/aad580f92f912c5b529963456f780764d0c197dc))
* align container image names across CI, chart, and Go code ([9cc053c](https://github.com/hortator-ai/Hortator/commit/9cc053cfc866ca5aab68fb250a765d6b87c78411))
* align k8s.io/api, client-go, controller-runtime with apimachinery v0.35.0 ([eb057bd](https://github.com/hortator-ai/Hortator/commit/eb057bd4acb17a3b619e4bfe0099ee3447d9ead9))
* align pr-check Go version to 1.25 ([e5b8635](https://github.com/hortator-ai/Hortator/commit/e5b8635c5d99b19ff3dfb3dd6c63ffbc24fcb9e7))
* **BUG-013:** use native sidecar for Presidio (K8s 1.28+) ([a7197c2](https://github.com/hortator-ai/Hortator/commit/a7197c26d3076eceaf1d605861c9cf42fc74a8bd))
* bump controller-gen to v0.18.0 in pr-check ([df1deee](https://github.com/hortator-ai/Hortator/commit/df1deee9e602e9e1ee50fe2b846cb70edfd22160))
* **ci:** bump Go to 1.25, fix lint errors, fix unit test, skip e2e in CI ([3498f3c](https://github.com/hortator-ai/Hortator/commit/3498f3c909a9bd3caf0fbac5f42bd01ae8355b0d))
* **ci:** fix goimports ordering, CRD timeout type, runtime Dockerfile path ([aceddf5](https://github.com/hortator-ai/Hortator/commit/aceddf5894124d6637e477a7970636a54b8aee74))
* **ci:** set golangci-lint go version to 1.24 (matching go.mod) ([1a0fe09](https://github.com/hortator-ai/Hortator/commit/1a0fe0993f923697fb1c408cbc2175b0207ded4a))
* clean up file-delivery.yaml manifest structure ([349c09b](https://github.com/hortator-ai/Hortator/commit/349c09bc8b4d1429b883b9ee93cb5718b3510b3a))
* clean up orphaned warm pool resources on operator restart ([5b6671e](https://github.com/hortator-ai/Hortator/commit/5b6671e37a2e1b8d77b9b87a75ea79c3f7a4cdea))
* configure agent images for E2E tests in Kind ([f93dc7f](https://github.com/hortator-ai/Hortator/commit/f93dc7f43eafd5488f72a624b6b59e7d067e7ce1))
* correct spawn CLI flags and JSON key mismatch in agentic runtime ([5bd89a9](https://github.com/hortator-ai/Hortator/commit/5bd89a9d5168271028b55bceb6f51a456854d55c))
* default all image tags to appVersion instead of latest ([95ded2a](https://github.com/hortator-ai/Hortator/commit/95ded2a928b713c3687c8968c841a7f34485bfb5))
* default all image tags to appVersion instead of latest ([44f4140](https://github.com/hortator-ai/Hortator/commit/44f4140a8cde3c6c424b52820538c6cade7fbb2c))
* handle errcheck for resp.Body.Close in qdrant client ([c96a896](https://github.com/hortator-ai/Hortator/commit/c96a896d3ab4571696e1d2d3f9a4a13590a8c1cd))
* improve phase 1 test manifests for reliable failure testing ([5f3aa53](https://github.com/hortator-ai/Hortator/commit/5f3aa53b9749a2544dc695be32b735558896c0c4))
* improve phase 1 test manifests for reliable failure testing ([c220277](https://github.com/hortator-ai/Hortator/commit/c220277324feb75b8058997e2c2f8bb8ff75bde1))
* include model and tier in result cache key ([c82fa44](https://github.com/hortator-ai/Hortator/commit/c82fa44a4f7dcd4c10161115a56c804ab396db13))
* inherit model spec from parent when child task has none ([05ebc9e](https://github.com/hortator-ai/Hortator/commit/05ebc9e746120c26973cb165acb9ebb1df247f5e))
* isolate test phases to prevent AgentPolicy cross-contamination ([01ed553](https://github.com/hortator-ai/Hortator/commit/01ed553e0a9e1d2c8d29c795e40a101404ec368f))
* kitchen-sink test uses self-generated code instead of external repo ([3500bd8](https://github.com/hortator-ai/Hortator/commit/3500bd86e4351d29d5c33ebe6b834c4014760520))
* **L6:** remove dead tier-to-model mapping from entrypoint.sh ([6cc3be1](https://github.com/hortator-ai/Hortator/commit/6cc3be169ffdef0e3a1c9887642572268cecfd45))
* lint errors in retry_test.go (unused func, goimports) ([dc5e3d9](https://github.com/hortator-ai/Hortator/commit/dc5e3d9821e74340b65828381b6a3003407f34bb))
* **lint:** goimports alignment in audit_test.go ([2b7effe](https://github.com/hortator-ai/Hortator/commit/2b7effe0238f101c0310b8e2c7fb2f61a9b4b587))
* **lint:** goimports grouping for local module imports ([4921bd8](https://github.com/hortator-ai/Hortator/commit/4921bd872ee4ccc0b186f328f44754df698a3072))
* **lint:** replace deprecated k8sfake.NewSimpleClientset with NewClientset ([317b8e5](https://github.com/hortator-ai/Hortator/commit/317b8e5572b2ec192a6a7b608d6dbacab042831b))
* **lint:** resolve all golangci-lint v2.8.0 issues + sync Makefile version ([e549417](https://github.com/hortator-ai/Hortator/commit/e5494175574b87cc70671c9b69fe884e85c5f501))
* make github-credentials optional in kitchen-sink test ([925027b](https://github.com/hortator-ai/Hortator/commit/925027bc07666653e176102829c0325b2b04db9e))
* nil-safe webhook check in Helm template ([6ba5d3e](https://github.com/hortator-ai/Hortator/commit/6ba5d3ee4c1e0b0fa1503baa46106d0a4531cb32))
* **operator:** cache ConfigMap with 30s TTL, add ±25% jitter to retry backoff ([1d71b75](https://github.com/hortator-ai/Hortator/commit/1d71b75bd37ccc4880c8be99cc07976114a0b7f6))
* **operator:** resolve P0 code review issues ([7b8de19](https://github.com/hortator-ai/Hortator/commit/7b8de19d0ff81607a87165fac86d6e051b8a4acf))
* **P0:** regen CRDs, inject apiKeyRef, remove hardcoded image ([4c70774](https://github.com/hortator-ai/Hortator/commit/4c70774dab614c92eeda95144c780d56ee5bdc37))
* **P1:** PVC outbox persistence, CRD consolidation, fix examples ([b9d54c2](https://github.com/hortator-ai/Hortator/commit/b9d54c28e53a6393f96dcec860546999b09a61cb))
* persist model inheritance and fix anonymizer endpoint ([#43](https://github.com/hortator-ai/Hortator/issues/43)) ([4180f50](https://github.com/hortator-ai/Hortator/commit/4180f50a47cc27fdb06f640477e56bf207effd95))
* quickstart roles.yaml wrong apiVersion and schema (BUG-003 missed) ([eeb0d25](https://github.com/hortator-ai/Hortator/commit/eeb0d25d5b226b5dee32a015a9e9ba634ec999b5))
* redact PII in runtime output via Presidio anonymize endpoint ([8dbe4dc](https://github.com/hortator-ai/Hortator/commit/8dbe4dcffccfcf35b5124cc2c811f3fa1fe66e76))
* redact PII in runtime output via Presidio anonymize endpoint ([e713972](https://github.com/hortator-ai/Hortator/commit/e713972e6c00dfa6f01297bbf24249daa3798a7e))
* reduce cleanup TTL defaults (completed=1h, failed=24h, cancelled=1h) ([579d747](https://github.com/hortator-ai/Hortator/commit/579d7478ed67103f4f472d3b7cb135cbfc5b3e7c))
* reduce cleanup TTL defaults to practical values ([8d4c847](https://github.com/hortator-ai/Hortator/commit/8d4c847794e5fc900c7d6ce067ccc4e500e0eed0))
* regenerate deepcopy to match controller-gen output ([2e3bdff](https://github.com/hortator-ai/Hortator/commit/2e3bdffdea0038240362a91ae2b551c15a48d9bd))
* regenerate deepcopy to match controller-gen output order ([1b0de6b](https://github.com/hortator-ai/Hortator/commit/1b0de6b40060cc50b6871e9bf28a03f4491254fa))
* regenerate deepcopy, CRDs and RBAC with controller-gen v0.18.0 ([aef9ef2](https://github.com/hortator-ai/Hortator/commit/aef9ef2c52c847e204b6b3c1a6039fbf2f332390))
* remove hardcoded storageClass, use cluster default ([97040c1](https://github.com/hortator-ai/Hortator/commit/97040c1e6d6d344bfe75d6f20d7819601c4b3755))
* remove unsupported fields from ClusterAgentRole test manifests ([6526bcb](https://github.com/hortator-ai/Hortator/commit/6526bcbd02cea425411ea0e05788be5e2a9fc9c3))
* resolve all golangci-lint issues across codebase ([7cb53d5](https://github.com/hortator-ai/Hortator/commit/7cb53d522e4655ed9c29c4ae5a5a1c826f750743))
* resolve golangci-lint errors (errcheck, goimports, gosimple) ([cbfce65](https://github.com/hortator-ai/Hortator/commit/cbfce65da10be2fb2d85f95c3c375215412b9846))
* resolve P1 code review issues ([02fc37d](https://github.com/hortator-ai/Hortator/commit/02fc37df5ce714c19fc9b8a3f2e4ce8f686d34ae))
* restore apiVersion declarations in corrupted test manifests ([23787ed](https://github.com/hortator-ai/Hortator/commit/23787ed35e9c06080f5d11c617e19a4f3673abe9))
* skip PVC owner reference when retain-pvc annotation is set ([30a9726](https://github.com/hortator-ai/Hortator/commit/30a9726df9a11d36679dd90d734d394286418f8a))
* update policy glob patterns to match new image paths ([09350b1](https://github.com/hortator-ai/Hortator/commit/09350b1cc179c70e023ff893df18c3ce38df0e8a))
* use correct runtime entrypoint for agentic tiers in warm pool ([6094f9d](https://github.com/hortator-ai/Hortator/commit/6094f9d7bca37fc2c951b2f7cd7f897c6ed1678a))
* use correct runtime entrypoint for agentic tiers in warm pool ([1e40da8](https://github.com/hortator-ai/Hortator/commit/1e40da892cbad1ba939dbfb2baca819be78aedb3))
* wire result cache check before pod creation and add retain CLI command ([2a5c52a](https://github.com/hortator-ai/Hortator/commit/2a5c52a50b0d51014548e3348f3a41327eb6130f))
* wire result cache config and add hortator retain CLI command ([407a195](https://github.com/hortator-ai/Hortator/commit/407a19536d5b3b66b7d6185dec86d145db274f18))

## [0.2.0](https://github.com/hortator-ai/Hortator/compare/v0.1.2...v0.2.0) (2026-02-14)


### Features

* add pluggable vector store abstraction with Qdrant implementation ([9e4622a](https://github.com/hortator-ai/Hortator/commit/9e4622a9abf97bf18d96bb832184d31f3d25db50))
* add pluggable vector store abstraction with Qdrant implementation ([74b58d4](https://github.com/hortator-ai/Hortator/commit/74b58d4e21c5d745004a36bf6aece814474e3580))
* add shell command filtering and read-only workspace to AgentPolicy ([bc994e2](https://github.com/hortator-ai/Hortator/commit/bc994e21710699833eea8d6ded17e62f14e3950f))
* add shell command filtering and read-only workspace via AgentPolicy ([38bd1b6](https://github.com/hortator-ai/Hortator/commit/38bd1b6b32b267f16e8f6dfff403bb41b0c91799))
* per-capability RBAC — split worker ServiceAccounts ([0f5be43](https://github.com/hortator-ai/Hortator/commit/0f5be436ae988aeba5d8551279454dbb25ded60d))
* redact PII in prompts and tool results before LLM submission ([#35](https://github.com/hortator-ai/Hortator/issues/35)) ([ab4d0f4](https://github.com/hortator-ai/Hortator/commit/ab4d0f4cf8d81a07676fcaba19726a33ed2949df))
* split worker ServiceAccount by spawn capability for least-privilege RBAC ([2974e45](https://github.com/hortator-ai/Hortator/commit/2974e45a1d90e257ade5a6c0e2dc41d6aa7407a3))
* validate child task capabilities are subset of parent ([#31](https://github.com/hortator-ai/Hortator/issues/31)) ([93a1a2d](https://github.com/hortator-ai/Hortator/commit/93a1a2d312b55d2bd5708344427bf57ff117aae2))


### Bug Fixes

* handle errcheck for resp.Body.Close in qdrant client ([5a677a7](https://github.com/hortator-ai/Hortator/commit/5a677a7549b4655d4a1484e9e0f129091c6e1aa5))
* regenerate deepcopy to match controller-gen output ([398b74a](https://github.com/hortator-ai/Hortator/commit/398b74acd802e1b5a031de6ae8f215d9800383db))

## [0.1.2](https://github.com/hortator-ai/Hortator/compare/v0.1.1...v0.1.2) (2026-02-14)


### Bug Fixes

* correct spawn CLI flags and JSON key mismatch in agentic runtime ([b71db5c](https://github.com/hortator-ai/Hortator/commit/b71db5c5a950fc8aae9161807b971c8b91ca19ad))
* isolate test phases to prevent AgentPolicy cross-contamination ([d20b738](https://github.com/hortator-ai/Hortator/commit/d20b73857f1df3bee2154df84be7305d353e71a9))
* redact PII in runtime output via Presidio anonymize endpoint ([428f1bc](https://github.com/hortator-ai/Hortator/commit/428f1bc09039daebedff2c4512dfbbca5671ee21))
* redact PII in runtime output via Presidio anonymize endpoint ([a4c03b3](https://github.com/hortator-ai/Hortator/commit/a4c03b3d7b54e2c6b4659b6483265d5a62442b2d))
* use correct runtime entrypoint for agentic tiers in warm pool ([ad421a3](https://github.com/hortator-ai/Hortator/commit/ad421a38257d32bd38ebe6950ad5e71c04a5c042))
* use correct runtime entrypoint for agentic tiers in warm pool ([b496b54](https://github.com/hortator-ai/Hortator/commit/b496b542aaa342316a51a61052d094156fc0dffc))
* wire result cache check before pod creation and add retain CLI command ([e223593](https://github.com/hortator-ai/Hortator/commit/e2235933f2c14f672f0001dbb88ab885bb3e9e58))
* wire result cache config and add hortator retain CLI command ([bab297d](https://github.com/hortator-ai/Hortator/commit/bab297d2f23e772d0f72d31294d4e6a488a20885))

## [0.1.1](https://github.com/hortator-ai/Hortator/compare/v0.1.0...v0.1.1) (2026-02-13)


### Bug Fixes

* add cleanup steps between test phases to prevent policy pollution ([c0aa2ba](https://github.com/hortator-ai/Hortator/commit/c0aa2ba74ef08d8e7654fda03a8a7045fef3bc5f))
* default all image tags to appVersion instead of latest ([2fcc8e4](https://github.com/hortator-ai/Hortator/commit/2fcc8e4b6f5204db484960e9938e18d419309294))
* default all image tags to appVersion instead of latest ([e48184d](https://github.com/hortator-ai/Hortator/commit/e48184dfbcf5e20276d4a5405840d415a2444b84))
* improve phase 1 test manifests for reliable failure testing ([6153dc2](https://github.com/hortator-ai/Hortator/commit/6153dc2efa9345fb419a7a74c9d0ca0a836019e5))
* improve phase 1 test manifests for reliable failure testing ([7e563d0](https://github.com/hortator-ai/Hortator/commit/7e563d0930b6379b6c39303bef94555a3ced711d))
* kitchen-sink test uses self-generated code instead of external repo ([4156ca5](https://github.com/hortator-ai/Hortator/commit/4156ca582c3fb7e8607aceba5fab950f3073de28))
* make github-credentials optional in kitchen-sink test ([0a1405b](https://github.com/hortator-ai/Hortator/commit/0a1405bb27d4d79c2eb96a43b28bd8e138a4c5de))
* reduce cleanup TTL defaults (completed=1h, failed=24h, cancelled=1h) ([e2a9544](https://github.com/hortator-ai/Hortator/commit/e2a9544cda403d89c18bef4b836011ce6852987a))
* reduce cleanup TTL defaults to practical values ([c13c18c](https://github.com/hortator-ai/Hortator/commit/c13c18caf0b0747eec79671bef127c2612ac7a4e))
* remove hardcoded storageClass, use cluster default ([3c39081](https://github.com/hortator-ai/Hortator/commit/3c3908174df3e71ce8db8491281413622ea29706))

## [0.1.0](https://github.com/hortator-ai/Hortator/compare/v0.0.1...v0.1.0) (2026-02-13)


### Features

* TUI single-pane view for logs, details, describe, summary ([19cfb6b](https://github.com/hortator-ai/Hortator/commit/19cfb6b62a1bacda682d5e521bc0c7fdf9b1a14a))
* TUI single-pane views for logs, details, describe, summary ([ec4a6c9](https://github.com/hortator-ai/Hortator/commit/ec4a6c94d52a6fa576469485f987143c25799455))


### Bug Fixes

* align container image names across CI, chart, and Go code ([f299b9f](https://github.com/hortator-ai/Hortator/commit/f299b9f246ab4e38fe9ceb6ab4d1235c2258643e))
* align container image names across CI, chart, and Go code ([ef40362](https://github.com/hortator-ai/Hortator/commit/ef403627c8e6a6c055d5f52aa27283a8e3400e0a))
* align pr-check Go version to 1.25 ([7cde320](https://github.com/hortator-ai/Hortator/commit/7cde320a0c4b6941911aa9ee98c58e3a01c411f9))
* bump controller-gen to v0.18.0 in pr-check ([565b4ba](https://github.com/hortator-ai/Hortator/commit/565b4ba488e68757bde9fa605424a0ff86ea87c0))
* regenerate deepcopy, CRDs and RBAC with controller-gen v0.18.0 ([32804cf](https://github.com/hortator-ai/Hortator/commit/32804cf81864b917e6982677c05c99e3d2577039))
* update policy glob patterns to match new image paths ([467457d](https://github.com/hortator-ai/Hortator/commit/467457d8ee9ddef0d13a5455c73d8a1d136ef765))
