package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Task 用户任务。待办查询高频：assignee+status、instance、token、due_at 均建索引。
type Task struct {
	ent.Schema
}

func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"),
		field.String("instance_id"),
		field.String("tenant_id").Default(""),
		field.String("node_id"),
		field.String("token_id"),
		field.String("name").Default(""),
		field.String("assignee"),
		field.String("status"),
		field.String("outcome").Default(""),
		field.Text("comment").Default(""),
		field.Time("due_at").Optional().Nillable(),
		field.Time("created_at"),
		field.Time("ended_at").Optional().Nillable(),
	}
}

func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("assignee", "status"),
		index.Fields("instance_id"),
		index.Fields("token_id"),
		index.Fields("status", "due_at"),
	}
}
