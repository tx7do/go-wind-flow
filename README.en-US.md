<div align="center">

<img src="docs/brand/flow-tile.svg" width="120" alt="GoWind Flow" />

# GoWind Flow

**The lightweight workflow engine of the GoWind family: persisted token state machine · BPMN 2.0 compatible · zero-dependency core**

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![BPMN](https://img.shields.io/badge/BPMN-2.0%20Compatible-4B8BBE)](./docs/bpmn_compat.md)
[![Scripts](https://img.shields.io/badge/go--scripts-9%20Engines-2E7D32)](https://github.com/tx7do/go-scripts)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen)](./engine_test.go)

**English** | [中文](./README.md) | [日本語](./README.ja-JP.md)

</div>

---

`go-wind-flow` is the workflow foundation of the [go-wind](https://github.com/tx7do/go-wind)
ecosystem, extracted and generalized from the approval-flow engines of
[go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im, and built to work hand in hand
with [go-scripts](https://github.com/tx7do/go-scripts) — **the engine owns the routing; scripts
own everything beyond the routing**.

It covers the commonly used subset of Flowable / Camunda / Activiti, providing an
approval / business-process foundation for Go private-deployment products (OA, ERP, CRM,
low-code platforms).

## Highlights

- **BPMN 2.0 compatible**: import and export BPMN XML via `format/bpmn` — deploy existing BPMN
  files as-is, and exported definitions open directly in bpmn-js / Camunda Modeler. Node types,
  gateway semantics and condition style all align with BPMN
  (see [docs/bpmn_compat.md](./docs/bpmn_compat.md))
- **Zero-rewrite migration of existing processes**: `format/windjson` reads the node_config JSON
  of go-wind-oa / go-wind-im (v2 graph format plus the legacy node array); the engine
  auto-detects the format by content (see [docs/oa_im_migration.md](./docs/oa_im_migration.md))
- **Persisted token state machine**: every transition is written to the store — crash-safe by
  design, no in-memory execution; definitions are immutable and versioned, in-flight instances
  are pinned to their deployed version
- **Zero-dependency core, brick-style SPIs**: the `flow` package imports no third-party library —
  storage, expressions, scripts, identity and formats are all SPIs wired in as needed
  (see [docs/architecture_design.md](./docs/architecture_design.md))
- **Pluggable script engines**: `contrib/goscripts` connects to 9 go-scripts engines
  (JS / Lua / Python / CEL / Expr / Go / WASM…), pure Go with zero CGO; `contrib/exprlang`
  provides the same evaluator behavior as go-wind-oa / go-wind-im in production
- **Enterprise semantics built in**: multi-tenancy, counter-signing (ALL) / any-signing (ANY),
  delegation, task forwarding, add-signing (dynamic approvers), return-to-node,
  due-time tracking, timer nodes (ISO8601 durations), variable scoping
  (token-local variables, per-branch isolation), fail-closed subprocess
  propagation, full history audit trail
- **Fail-closed by default**: a failed condition evaluation never takes a branch, an unresolved
  assignee never creates a task, script sandboxes expose only whitelisted APIs — every
  uncertain state is rejected, never waved through

## Where to start

- **Just want to try it**: jump to [Quick start](#quick-start) and run the leave-approval
  example end to end ([examples/leave-approval.bpmn.xml](./examples/leave-approval.bpmn.xml)).
- **Want the full picture**: read the [docs index](./docs/README.md) — reading paths per role
  (adopter / host integrator / migrator / contributor), with one authoritative reference doc
  per topic.
- **"Can I import my BPMN?"**: check the
  [BPMN support matrix](./docs/bpmn_compat.md#2-支持矩阵m0).

## Repositories

| Platform | URL |
|----------|-----|
| GitHub | <https://github.com/tx7do/go-wind-flow> |
| Gitee | syncing |
| AtomGit | syncing |

## Quick start

### Requirements

| Tool | Version |
|------|---------|
| Go | 1.25+ (core / embedded; `api` submodule 1.26+) |
| (optional) go-scripts | sibling checkout or fetched from GitHub — needed for script tasks / CEL·Expr evaluation |

### Install

```bash
go get github.com/tx7do/go-wind-flow
```

### Leave approval in three steps

```go
package main

import (
	"context"
	"os"

	"github.com/tx7do/go-wind-flow/flow"
	bpmn "github.com/tx7do/go-wind-flow/format/bpmn"
	"github.com/tx7do/go-wind-flow/store/memory"
)

func main() {
	ctx := context.Background()
	eng := flow.New(
		flow.WithStore(memory.New()),        // swap for an ent-backed store in production
		flow.WithImporters(bpmn.Importer{}), // BPMN XML importer
	)

	// 1. Deploy (immutable, auto-versioned)
	bpmnData, _ := os.ReadFile("examples/leave-approval.bpmn.xml")
	_, _ = eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "leave", Data: bpmnData})

	// 2. Start an instance (advances synchronously to the first blocking point)
	inst, tasks, _ := eng.Start(ctx, flow.StartRequest{
		TenantID:  "t1",
		Key:       "leave",
		StarterID: "u_emp",
		Variables: flow.Vars{"days": 5},
	})

	// 3. Complete tasks (ALL/ANY/reject semantics are driven by the definition)
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove,
	})
	_ = inst
}
```

### Production storage

The engine ships with `store/memory` (embedded / for tests). For production, use the
ent or GORM store — separate submodules, pulled in only when needed:

```bash
go get github.com/tx7do/go-wind-flow/store/ent    # ent (GoWind family convention)
go get github.com/tx7do/go-wind-flow/store/gorm   # GORM (MySQL/PostgreSQL/SQLite)
```

```go
st := entstore.New(client) // schema: client.Schema.Create(ctx) or your migrations
eng := flow.New(flow.WithStore(st), flow.WithImporters(bpmn.Importer{}))

// Wrap engine entry points in a transaction (WithTx pattern; the GORM store
// exposes the same method with the same signature)
_ = st.WithTx(ctx, func(ctx context.Context) error {
	return eng.CompleteTask(ctx, req)
})
```

### Protobuf service contract

For gRPC integration (embedding the service or talking to a standalone flow
center), pull in the `api` submodule — four services (FlowDefinition / FlowInstance /
FlowTask / FlowHistory) as a **pure gRPC contract with no HTTP annotations**
(family convention: HTTP routes are defined per-host in the BFF layer), and the
same paging protocol as the GoWind family (go-crud `PagingRequest`):

```bash
go get github.com/tx7do/go-wind-flow/api
```

The protos are also published to the BSR — non-Go consumers (TS / Dart, …) and
cross-repo proto imports can depend directly:

```bash
buf dep buf.build/go-wind/go-wind-flow
```

Engine domain structs stay plain Go (domain model ≠ transport DTO); the kratos
service wrapper is on the M1 roadmap — contract first, hosts can wire against the
generated `flowv1` service interfaces today. Regenerate & publish:
`make proto` + `buf push --label <version>` (aligned with git tags).

### Run the tests

```bash
go test ./...        # engine + format adapters (incl. BPMN round-trip tests)
make tidy            # tidy all three go.mod files (core / contrib/exprlang / contrib/goscripts)
```

## Architecture at a glance

```
┌──────────────────────────────────────────────────────────────┐
│  Host apps (go-wind-oa / go-wind-im / go-wind-erp / CRM ...) │
└──────┬───────────────────────────────┬───────────────────────┘
       │ engine API                     │ SPI implementations (wire as needed)
┌──────▼───────────────────────────────▼───────────────────────┐
│                      flow (core, zero-dep)                   │
│  Engine / Deploy·Start·CompleteTask·Withdraw                 │
│  walker (token-advance kernel)   validate (deploy checks)    │
│  ── SPIs ──────────────────────────────────────────────────  │
│  Store  ExpressionEvaluator  ScriptRunner  IdentityProvider  │
│  Importer/Exporter  Listener  Clock/IDGen                    │
└──────▲──────────▲──────────▲──────────▲──────────────────────┘
       │          │          │          │
 store/           contrib/   contrib/   format/
 memory·ent·gorm  exprlang   goscripts  bpmn · windjson
 (embedded/prod)  (expr-lang) (9 engines) (BPMN XML · legacy JSON)
```

## Division of labor with go-scripts

| Scenario | Use | Notes |
|---|---|---|
| Gateway conditions, dynamic assignees | built-in `SimpleEvaluator`, or `contrib/exprlang` / go-scripts CEL·Expr | deterministic, side-effect free, fail-closed |
| Script tasks, listener logic | `contrib/goscripts` (JS/Lua/Python/Go/WASM…) | Turing-complete, sandboxed, context-deadlined |
| High-security scenarios | go-scripts WASM (wazero) | strong isolation |

## Where it sits in the GoWind family

| Project | Relationship |
|------|------|
| [go-wind](https://github.com/tx7do/go-wind) | Microservice kernel: this engine follows its brick-box philosophy (zero hidden magic, SPI assembly) |
| [go-wind-admin](https://github.com/tx7do/go-wind-admin) | Enterprise admin scaffold: this engine can serve as its approval-flow foundation |
| [go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im | Original hosts of the approval engine; migration guide in [docs/oa_im_migration.md](./docs/oa_im_migration.md) |
| [go-scripts](https://github.com/tx7do/go-scripts) | Embedded scripting framework: implements script tasks and expression evaluation |

## Docs

Entry point: [docs/README.md](./docs/README.md)

| Doc | Topic |
|------|------|
| [architecture_design.md](./docs/architecture_design.md) | Engine architecture: token state machine, transaction boundaries, SPI contracts, fail-closed semantics |
| [bpmn_compat.md](./docs/bpmn_compat.md) | BPMN compatibility: support matrix, assignee extension convention, bpmn-js / Camunda interop |
| [designer_contract.md](./docs/designer_contract.md) | Designer contract: designer-json-v1 document shape and designer↔server interaction protocol |
| [oa_im_migration.md](./docs/oa_im_migration.md) | Migration: engine comparison, strangler path, legacy definition compatibility, behavior diff list |
| [roadmap.md](./docs/roadmap.md) | Roadmap: M0 core (current) → M1 production → M2 BPMN expansion → M3 ecosystem → M4 standalone |

## Roadmap

Currently at **M0 (core skeleton)** — engine kernel, format adapters and the full SPI set are in
place and covered by end-to-end tests. Next stop is M1 productionization (ent store, kratos
service wrapper, go-wind-ui designer contract); see
[docs/roadmap.md](./docs/roadmap.md) for all milestones.

## License

[MIT](./LICENSE)
