# Design: File Delivery & RAG Integration

**Status:** Proposal  
**Date:** 2026-02-10  
**Author:** Daemon + M

## Problem

Certain agent tasks require more than a text prompt. A Tribune tasked with "analyse this contract against applicable laws, code, and company policies" needs two things:

1. **The contract itself** — a file uploaded by the user and delivered into the agent's `/inbox/`.
2. **A large reference corpus** — thousands of legal documents, building codes, and policies that the agent can search during execution.

Today, Hortator has no mechanism for either. The gateway, SDKs, and CLI only accept plain-text prompts. There is no integration point for vector stores or retrieval-augmented generation (RAG).

This document defines the scope boundary: what Hortator should own, what it should not, and why.

## Scope Boundary

### In Scope: File Delivery to Agents

Hortator owns the lifecycle of agent pods and their filesystem. Delivering user-provided files into `/inbox/` is a natural extension of the existing task creation flow — it is the same class of problem as writing `task.json`.

**What this means in practice:**

- The gateway accepts file attachments alongside the prompt (inline base64 or pre-uploaded file references).
- The operator writes those files into `/inbox/` via the init container (new pods) or exec injection (warm pool pods), just as it does for `task.json` today.
- The SDKs and CLI expose file parameters so every client surface can attach files to tasks.

This is a delivery mechanism. Hortator moves bytes from the caller into the agent's filesystem. It does not interpret, parse, or index those files.

### In Scope: Agent Access to Vector Stores

Hortator already has a capabilities model — agents declare what tools they can use (`shell`, `web-fetch`, `spawn`), and the operator configures the pod accordingly (NetworkPolicies, sidecars, environment variables). RAG fits this pattern exactly.

An agent with a `rag` capability would receive:
- Network access to the vector store endpoint.
- Connection configuration (endpoint URL, collection name, auth credentials) injected via environment variables or a mounted config file.
- Optionally, a sidecar or MCP server that exposes a search tool to the agent.

The agent uses this tool during execution to query the reference corpus. From Hortator's perspective, the vector store is just another external service — like an LLM endpoint or a web API.

**Configuration would live in Helm values and/or AgentRole:**

```yaml
# Helm values (cluster-wide defaults)
capabilities:
  rag:
    endpoint: "http://milvus.vector-system:19530"
    defaultCollection: "legal-corpus"
    secretRef: milvus-credentials

# AgentRole (per-role overrides)
apiVersion: core.hortator.ai/v1alpha1
kind: ClusterAgentRole
metadata:
  name: legal-analyst
spec:
  tools: [shell, rag]
  rag:
    collection: "legal-documents-eu"
    topK: 20
```

### In Scope: Knowledge Graduation (Enterprise)

The architecture already defines a storage lifecycle where stale retained PVCs can be "graduated" into a vector store — the agent's *outputs* (analyses, summaries, extracted knowledge) get embedded and indexed so future agents can discover them. This is an enterprise feature on the roadmap.

This is distinct from corpus ingestion. Knowledge graduation indexes what agents *produce*. Corpus ingestion indexes what humans *provide*.

### Out of Scope: Corpus Ingestion

Populating a vector store with a reference corpus is a **data pipeline problem**, not an orchestration problem. It involves:

- **Document parsing** — extracting text from PDFs, DOCX, HTML, scanned images (OCR).
- **Chunking strategies** — splitting documents by section, paragraph, or overlapping windows. The optimal strategy varies by domain (legal vs. medical vs. code).
- **Embedding generation** — calling an embedding model (OpenAI `text-embedding-3-large`, local models via Ollama, etc.) to convert chunks into vectors.
- **Index management** — creating collections, configuring partitions, setting up metadata schemas, managing index types (IVF, HNSW).
- **Ongoing synchronization** — detecting new/updated/deleted documents and keeping the index current.
- **Domain-specific preprocessing** — legal citation extraction, code symbol indexing, medical terminology normalization.

This is a well-served problem space. Tools like LlamaIndex, LangChain document loaders, Unstructured.io, and purpose-built ingestion services handle this with mature, battle-tested implementations. Hortator adding its own ingestion layer would be:

1. **Scope creep** — Hortator is an orchestrator, not a data pipeline. The same argument that separates Hortator from LangGraph/CrewAI applies here: Hortator is infrastructure, not application logic.
2. **Redundant** — every organization with a vector store already has (or needs) an ingestion pipeline. Hortator duplicating this adds complexity without unique value.
3. **Domain-dependent** — chunking strategies, metadata schemas, and preprocessing steps vary wildly between legal, medical, code, and other domains. A general-purpose solution is either too generic to be useful or too complex to maintain.

### Why This Boundary Matters

The boundary follows Hortator's core design principle: **orchestrate agents, don't be the agent**. Hortator manages pods, lifecycles, budgets, and security. It does not parse PDFs, generate embeddings, or manage vector indices — just as it does not write code, review contracts, or analyse data.

The vector store is a tool. Hortator ensures agents can reach and use that tool. Populating the tool is someone else's job.

## Scope Summary

| Concern | In scope? | Why |
|---------|-----------|-----|
| Delivering uploaded files to agent `/inbox/` | **Yes** | Natural extension of task creation — same class as `task.json` delivery |
| Agent querying a vector store during execution | **Yes** | Fits the capabilities model — vector store is an external tool like any other |
| Graduating agent outputs to vector store | **Yes** (enterprise) | Already designed in the storage lifecycle |
| Bulk corpus ingestion into vector store | **No** | Data pipeline problem, well-served by existing tools, domain-dependent |
| Document parsing, chunking, embedding | **No** | Application logic, not orchestration infrastructure |
| Vector index schema management | **No** | Domain-specific, tightly coupled to the corpus and query patterns |

## Food for Thought: Orchestrating Ingestion with Hortator

There is an interesting meta-pattern worth noting. While Hortator should not *contain* ingestion logic, it could *orchestrate* an ingestion pipeline as a regular task hierarchy.

A tribune could receive "ingest these 500 contracts into the legal knowledge base" and decompose it into centurion batches, each spawning legionaries that:

1. Read documents from a shared volume or object store.
2. Use external tools (Unstructured for parsing, an embedding API, a Milvus client) to process and index them.
3. Report progress and errors through the standard `/outbox/result.json` mechanism.

This pattern means an organization does not need a separate orchestration system for their data pipeline. Hortator handles scheduling, parallelism, retries, budget tracking, and monitoring — the same value it provides for any other agent workload.

The key distinction: Hortator orchestrates *agents that run ingestion tools*. It does not embed ingestion logic into the operator or gateway. The ingestion knowledge lives in the agent image and prompt, not in Hortator's codebase.

This is the same relationship Hortator has with code review, test execution, or document analysis — it runs agents that do those things, without knowing how to do them itself.

## Implementation Considerations

### File Delivery via the OpenAI-Compatible API

The OpenAI Chat Completions API already supports files in message content arrays via a `FileContentPart` type:

```json
{
  "role": "user",
  "content": [
    { "type": "text", "text": "Analyse this contract against applicable laws" },
    {
      "type": "file",
      "file": {
        "file_data": "<base64-encoded content>",
        "filename": "contract.pdf"
      }
    }
  ]
}
```

OpenAI also supports a two-step upload: `POST /v1/files` returns a `file_id`, which is then referenced in the message. This avoids large JSON payloads.

Adopting this format means any standard OpenAI SDK (`openai` Python/Node package) could send files to Hortator without needing the Hortator-specific SDK — preserving the drop-in replacement property of the API.

### Changes Required

**Gateway** — The `Message` type currently defines `Content` as a plain `string`. It must support the OpenAI union type: either a string or an array of typed content parts (text, file, image). The gateway must extract file content parts, write them to `/inbox/`, and pass the text parts as the prompt.

**Python SDK** — `Message.content` must accept `str | list[ContentPart]`. Convenience methods should accept a `files` parameter.

**TypeScript SDK** — Same pattern. `Message.content` becomes `string | ContentPart[]`.

**CLI** — The `spawn` command needs a `--file` flag (repeatable). Since the CLI talks directly to the Kubernetes API (not the gateway), files must either be embedded in the AgentTask spec (limited by etcd's ~1.5MB object size limit), stored in a ConfigMap, or uploaded to a pre-provisioned PVC. The gateway path avoids this constraint.

**Operator** — The init container and warm pool injection must be extended to write files from the task spec (or referenced storage) into `/inbox/` alongside `task.json`.

### Size Constraints

- **Inline base64** in the OpenAI-compatible request: practical up to ~1-2MB per file (JSON payload size).
- **Pre-upload via `/v1/files`**: the gateway stores the file (in a PVC, object storage, or temp dir) and references it by ID during task creation. Required for larger files.
- **CRD path** (CLI direct-to-K8s): etcd enforces a ~1.5MB limit per object. Files larger than a few hundred KB must use an alternative storage path.
