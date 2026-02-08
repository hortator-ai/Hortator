# Go-to-Market Plan

*Internal document — not published to docs site.*

## Phase 1: Pre-Launch (build hype before repo goes public)

**Manifesto blog post** — thesis, not product announcement:
- Title ideas: "AI Agent Frameworks Are Building on Sand" / "Why AI Agents Need Kubernetes-Native Infrastructure"
- Position the problem (agents in single containers = no isolation, no governance)
- Reference competitive landscape (LangGraph/CrewAI/AutoGen are frameworks, not infra)
- End with "we're building this" + link to waitlist/repo
- Publish on: personal blog, dev.to, Medium, Hacker News

**Twitter/X thread** — 10-tweet thread with architecture diagram. Tag AI/K8s influencers.

**Reddit** — r/kubernetes + r/LocalLLaMA

## Phase 2: Launch (when MVP works end-to-end)

**GitHub repo goes public** with:
- Polished README ✅
- Working `helm install` → agent runs → completes task
- Examples directory ✅
- Docs site (MkDocs Material) ✅
- `CONTRIBUTING.md`
- Short demo video/GIF in README (30 seconds, `asciinema` recording)

**Hacker News "Show HN"** — #1 channel for K8s/infra tools:
> Show HN: Hortator – Kubernetes-native orchestration for autonomous AI agents

**Product Hunt** — secondary, reaches non-K8s AI audience.

**CNCF landscape submission** — apply for listing under AI/ML or Orchestration. Instant credibility.

## Phase 3: Community Building

- **Discord server** — early adopters, feedback, support
- **"Awesome Hortator" repo** — example roles, tasks, community runtimes
- **Conference talks** — KubeCon CFP (submit 6 months ahead), local meetups sooner
- **Integration blog posts** — "How to run CrewAI agents on Hortator", "LangGraph + Hortator"
- **YouTube** — 5-min "from zero to agent hierarchy" tutorial

## Phase 4: Enterprise Pipeline

- Trigger: OSS traction (stars, contributors, enterprise questions in Discord)
- Landing page at `hortator.io` with OSS + Enterprise tiers
- "Book a demo" for enterprise features
- Target: DevOps/Platform teams at companies using K8s + AI

## Timeline

| When | What |
|------|------|
| Now | Finish MVP implementation |
| Week 1-2 | Write manifesto blog post, record demo |
| Week 2 | Repo public, docs live, Show HN |
| Week 3-4 | Community seeding (Reddit, Twitter, Discord) |
| Month 2-3 | Conference CFPs, integration blog posts |
| Month 3+ | Enterprise conversations |

## Launch Checklist

- [ ] 10-20 GitHub stars before Show HN (share with friends/colleagues first)
- [ ] ASCII/GIF demo in README (`asciinema`)
- [ ] One-liner install works flawlessly
- [ ] Docs site live at docs.hortator.io
- [ ] CONTRIBUTING.md + CODE_OF_CONDUCT.md
- [ ] LICENSE file (MIT + enterprise notice)
- [ ] Discord server created
- [ ] Manifesto blog post drafted
- [ ] Show HN post text prepared
- [ ] Twitter thread drafted
