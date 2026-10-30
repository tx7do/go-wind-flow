package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-wind-flow/flow"
)

// HistoryEvent 流转历史（append-only）。
type HistoryEvent struct {
	ent.Schema
}

func (HistoryEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"),
		field.String("instance_id"),
		field.String("type"),
		field.String("node_id").Default(""),
		field.String("task_id").Default(""),
		field.String("actor_id").Default(""),
		field.JSON("payload", flow.Vars{}),
		field.Time("created_at"),
	}
}

func (HistoryEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("instance_id", "created_at"),
	}
}
