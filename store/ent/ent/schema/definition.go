package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-wind-flow/flow"
)

// Definition 流程定义（不可变，按 tenant+key+version 唯一）。
// 实体 ID 为复合键 "tenantID|key|version"，Save 即幂等 upsert。
type Definition struct {
	ent.Schema
}

func (Definition) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable(),
		field.String("tenant_id").Default(""),
		field.String("def_key"),
		field.String("name").Default(""),
		field.Int("version"),
		field.String("format").Default(""),
		field.Text("source").Default(""),
		field.JSON("nodes", map[string]*flow.Node{}),
		field.JSON("edges", []*flow.Edge{}),
		field.Time("created_at"),
	}
}

func (Definition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "def_key", "version").Unique(),
		index.Fields("tenant_id", "def_key"),
	}
}
