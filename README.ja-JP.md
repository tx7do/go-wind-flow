<div align="center">

<img src="docs/brand/flow-tile.svg" width="120" alt="GoWind Flow" />

# GoWind Flow

**GoWind ファミリーの軽量ワークフローエンジン：永続化トークンステートマシン · BPMN 2.0 互換 · コア依存ゼロ**

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![BPMN](https://img.shields.io/badge/BPMN-2.0%20Compatible-4B8BBE)](./docs/bpmn_compat.md)
[![Scripts](https://img.shields.io/badge/go--scripts-9%20Engines-2E7D32)](https://github.com/tx7do/go-scripts)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen)](./engine_test.go)

[English](./README.en-US.md) | [中文](./README.md) | **日本語**

</div>

---

`go-wind-flow` は [go-wind](https://github.com/tx7do/go-wind) エコシステムのワークフロー基盤です。
[go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im の承認フローエンジンから抽出・汎用化し、
[go-scripts](https://github.com/tx7do/go-scripts) と連携して動作します。**エンジンはフローの遷移を、
スクリプトは遷移以外のロジックを担います。**

Java 界の Flowable / Camunda / Activiti の常用サブセットをカバーし、OA・ERP・CRM・ローコード
プラットフォームといった Go 製プライベートデプロイ市場向けの承認フロー / 業務フロー基盤を提供します。

## 主な特長

- **BPMN 2.0 互換**：`format/bpmn` により BPMN XML を双方向に変換——既存の BPMN ファイルをそのまま
  デプロイでき、エクスポート結果は bpmn-js / Camunda Modeler で直接開けます。ノード型・ゲートウェイ
  の意味論・条件式の書式は BPMN と整合（対応表は [docs/bpmn_compat.md](./docs/bpmn_compat.md)）
- **既存プロセスの無書き換え移行**：`format/windjson` が go-wind-oa / go-wind-im の node_config JSON
  （v2 グラフ形式＋旧ノード配列）をそのまま読み込みます。フォーマットは内容から自動判定
  （移行ガイドは [docs/oa_im_migration.md](./docs/oa_im_migration.md)）
- **永続化駆動のトークンステートマシン**：遷移のたびにストアへ書き込むためクラッシュセーフ。
  定義は不変・バージョン管理され、実行中のインスタンスはデプロイ済みバージョンに固定されます
- **コア依存ゼロ、積み木式 SPI**：`flow` パッケージはサードパーティに依存しません。ストア・式評価・
  スクリプト・ID・フォーマットはすべて SPI で、必要なものだけ組み立てます
  （契約は [docs/architecture_design.md](./docs/architecture_design.md)）
- **スクリプトエンジンは自由に差し替え可能**：`contrib/goscripts` で go-scripts の 9 種類のエンジン
  （JS / Lua / Python / CEL / Expr / Go / WASM…、純 Go・CGO ゼロ）に接続。`contrib/exprlang` は
  go-wind-oa / go-wind-im と同一の評価挙動を提供します
- **エンタープライズ機能を内蔵**：マルチテナント、全員承認（ALL）/ 一名承認（ANY）、代理依頼、
  タスク転送、加籤（動的承認者追加）、指定ノードへの差し戻し、期限管理、タイマーノード
  （ISO8601）、変数スコープ（トークン単位のローカル変数・分岐毎の分離）、
  サブフロー失敗時の fail-closed 伝播、完全な履歴監査
- **fail-closed 設計**：条件評価の失敗で分岐を誤らない、承認者の解決に失敗したらタスクを作らない、
  スクリプトはホワイトリスト API のみ——不確定な状態はすべて拒否します

## はじめに

- **まず動かす**：[クイックスタート](#クイックスタート)へ。休暇承認サンプル
  （[examples/leave-approval.bpmn.xml](./examples/leave-approval.bpmn.xml)）を一通り動かせます。
- **仕組みを理解する**：[ドキュメント索引](./docs/README.md)——役割別（採用者 / ホスト開発者 /
  移行担当 / コントリビューター）の読書パスつき。
- **「自分の BPMN を取り込める？」**：[BPMN 対応表](./docs/bpmn_compat.md#2-支持矩阵m0)を確認。

## リポジトリ

| プラットフォーム | URL |
|------|------|
| GitHub | <https://github.com/tx7do/go-wind-flow> |
| Gitee | 同期中 |
| AtomGit | 同期中 |

## クイックスタート

### 要件

| ツール | バージョン |
|------|------|
| Go | 1.25+（コア / 組込み；`api` サブモジュールは 1.26+） |
| （任意）go-scripts | スクリプトタスク / CEL·Expr 評価を使う場合に必要 |

### インストール

```bash
go get github.com/tx7do/go-wind-flow
```

### 休暇承認を 3 ステップで

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
		flow.WithStore(memory.New()),        // 本番は ent 実装へ
		flow.WithImporters(bpmn.Importer{}), // BPMN XML インポータ
	)

	// 1. デプロイ（不変・自動バージョン管理）
	bpmnData, _ := os.ReadFile("examples/leave-approval.bpmn.xml")
	_, _ = eng.Deploy(ctx, flow.DeployRequest{TenantID: "t1", Key: "leave", Data: bpmnData})

	// 2. インスタンス開始（最初のブロック点まで同時進行）
	inst, tasks, _ := eng.Start(ctx, flow.StartRequest{
		TenantID:  "t1",
		Key:       "leave",
		StarterID: "u_emp",
		Variables: flow.Vars{"days": 5},
	})

	// 3. タスク完了（ALL/ANY/却下の意味論は定義が決める）
	_ = eng.CompleteTask(ctx, flow.CompleteTaskRequest{
		TaskID: tasks[0], ActorID: "boss", Outcome: flow.OutcomeApprove,
	})
	_ = inst
}
```

### 本番用ストレージ

エンジンには `store/memory`（組込み / テスト用）が付属します。本番は ent または
GORM 実装を利用します（独立サブモジュール、必要な時だけ依存に追加）：

```bash
go get github.com/tx7do/go-wind-flow/store/ent    # ent（GoWind ファミリーの慣例）
go get github.com/tx7do/go-wind-flow/store/gorm   # GORM（MySQL/PostgreSQL/SQLite）
```

```go
st := entstore.New(client) // テーブル作成：client.Schema.Create(ctx) またはホスト側マイグレーション
eng := flow.New(flow.WithStore(st), flow.WithImporters(bpmn.Importer{}))

// エンジン入口をトランザクションで包む（WithTx パターン、nest 安全）
_ = st.WithTx(ctx, func(ctx context.Context) error {
	return eng.CompleteTask(ctx, req)
})
```

### Protobuf サービス契約

gRPC で統合する場合（サービスの埋め込み or 独立フローセンター）は `api`
サブモジュールを利用します——FlowDefinition / FlowInstance / FlowTask / FlowHistory
の 4 サービス、**HTTP アノテーションなしの純 gRPC 契約**（ファミリーの慣例：
HTTP ルートは各ホストの BFF レイヤーで定義）、ページングは
go-wind ファミリーと共通（go-crud `PagingRequest`）：

```bash
go get github.com/tx7do/go-wind-flow/api
```

proto は BSR にも公開しています——Go 以外の利用者（TS / Dart など）や他リポジトリ
からの proto 参照はそのまま依存に追加できます：

```bash
buf dep buf.build/go-wind/go-wind-flow
```

エンジンのドメイン構造はプレーンな Go のまま（ドメインモデル ≠ 転送 DTO）。
kratos サービスラッパーは M1 ロードマップ上——契約先行のため、ホストは生成済みの
`flowv1` サービスインターフェースに今から接続できます。再生成と公開：
`make proto` + `buf push --label <バージョン>`（git tag と対応）。

### テスト実行

```bash
go test ./...        # コア＋フォーマットアダプタ（BPMN 往復テスト含む）
make tidy            # 3 つの go.mod を順に tidy
```

## アーキテクチャ概要

```
┌──────────────────────────────────────────────────────────────┐
│  ホストアプリ（go-wind-oa / go-wind-im / go-wind-erp / CRM）  │
└──────┬───────────────────────────────┬───────────────────────┘
       │ エンジン API                    │ SPI 実装（必要に応じて組み立て）
┌──────▼───────────────────────────────▼───────────────────────┐
│                  flow（コア、依存ゼロ）                        │
│  Engine / Deploy·Start·CompleteTask·Withdraw                 │
│  walker（トークン推進カーネル）   validate（デプロイ検証）     │
│  ── SPI ──────────────────────────────────────────────────   │
│  Store  ExpressionEvaluator  ScriptRunner  IdentityProvider  │
│  Importer/Exporter  Listener  Clock/IDGen                    │
└──────▲──────────▲──────────▲──────────▲──────────────────────┘
       │          │          │          │
 store/           contrib/   contrib/   format/
 memory·ent·gorm  exprlang   goscripts  bpmn · windjson
 （組込み/本番）  (expr-lang) (9 種類)   (BPMN XML · 既存 JSON)
```

## go-scripts との分担

| シナリオ | 使用するもの | 説明 |
|---|---|---|
| ゲートウェイ条件・動的承認者 | 内蔵 `SimpleEvaluator`、または `contrib/exprlang` / go-scripts CEL·Expr | 決定的・副作用なし・fail-closed |
| スクリプトタスク・リスナー | `contrib/goscripts`（JS/Lua/Python/Go/WASM…） | チューリング完全、サンドボックス＋ context による時間制御 |
| 高セキュリティ | go-scripts の WASM（wazero） | 強分離 |

## GoWind ファミリーでの位置づけ

| プロジェクト | 関係 |
|------|------|
| [go-wind](https://github.com/tx7do/go-wind) | マイクロサービスカーネル：本エンジンはその「積み木箱」設計思想を受け継ぐ |
| [go-wind-admin](https://github.com/tx7do/go-wind-admin) | エンタープライズ管理画面スキャフォールド：承認フロー基盤として利用可能 |
| [go-wind-oa](https://github.com/tx7do/go-wind-oa) / go-wind-im | 承認エンジンの元ホスト。移行は [docs/oa_im_migration.md](./docs/oa_im_migration.md) |
| [go-scripts](https://github.com/tx7do/go-scripts) | 組込みスクリプトフレームワーク：スクリプトタスク・式評価を担当 |

## ドキュメント

索引：[docs/README.md](./docs/README.md)

| ドキュメント | 内容 |
|------|------|
| [architecture_design.md](./docs/architecture_design.md) | エンジン設計：トークンステートマシン、トランザクション境界、SPI 契約、fail-closed 意味論 |
| [bpmn_compat.md](./docs/bpmn_compat.md) | BPMN 互換：対応表、承認者拡張の約定、bpmn-js / Camunda との相互運用 |
| [designer_contract.md](./docs/designer_contract.md) | デザイナー契約：designer-json-v1 のドキュメント構造とデザイナー↔サーバー連携 |
| [oa_im_migration.md](./docs/oa_im_migration.md) | 移行：エンジン比較、ストラングラーパス、既存定義の互換、挙動差分 |
| [roadmap.md](./docs/roadmap.md) | ロードマップ：M0 コア（現在）→ M1 本番化 → M2 BPMN 拡充 → M3 エコシステム → M4 単体配布 |

## ロードマップ

現在は **M0（コア骨格）**——エンジンカーネル・フォーマットアダプタ・SPI 一式が揃い、
E2E テストを通過しています。次は M1 本番化（ent ストア、kratos サービスラッパー、
go-wind-ui デザイナー契約）。詳細は [docs/roadmap.md](./docs/roadmap.md)。

## ライセンス

[MIT](./LICENSE)
