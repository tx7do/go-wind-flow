package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-wind-flow/flow"
)

// Instance 流程实例。variables 存 JSON；状态以字符串存（校验在引擎层）。
type Instance struct {
	ent.Schema
}

func (Instance) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"),
		field.String("tenant_id").Default(""),
		field.String("definition_key"),
		field.Int("definition_version"),
		field.String("business_key").Default(""),
		field.String("business_type").Default(""),
		field.String("starter_id").Default(""),
		field.String("status"),
		field.JSON("variables", flow.Vars{}),
		field.String("parent_instance_id").Default(""),
		field.String("parent_token_id").Default(""),
		field.Time("created_at"),
		field.Time("ended_at").Optional().Nillable(),
	}
}

func (Instance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("tenant_id"),
		index.Fields("parent_instance_id"),
	}
}
