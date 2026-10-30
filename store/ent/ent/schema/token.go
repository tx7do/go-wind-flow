package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Token 执行令牌（go-wind-flow 新增实体，旧引擎没有）。
type Token struct {
	ent.Schema
}

func (Token) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"),
		field.String("instance_id"),
		field.String("node_id"),
		field.String("status"),
		field.String("parent_id").Default(""),
		field.String("child_inst_id").Default(""),
		field.Time("created_at"),
	}
}

func (Token) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("instance_id", "node_id", "status"),
	}
}
