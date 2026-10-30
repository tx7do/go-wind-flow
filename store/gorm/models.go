package gormstore

import (
	"time"

	"github.com/tx7do/go-wind-flow/flow"
)

// GORM 模型与 flow 运行时结构一一对应；JSON 复杂字段用 gorm serializer 落 TEXT。
// 表名由 GORM 复数化：definitions / instances / tokens / tasks / history_events。

type Definition struct {
	ID        string                `gorm:"primaryKey;size:191"` // tenantID|key|version
	TenantID  string                `gorm:"size:64;uniqueIndex:ux_def,priority:1"`
	DefKey    string                `gorm:"size:128;uniqueIndex:ux_def,priority:2"`
	Version   int                   `gorm:"uniqueIndex:ux_def,priority:3"`
	Name      string                `gorm:"size:255"`
	Format    string                `gorm:"size:32"`
	Source    string                `gorm:"type:text"`
	Nodes     map[string]*flow.Node `gorm:"serializer:json"`
	Edges     []*flow.Edge          `gorm:"serializer:json"`
	CreatedAt time.Time
}

func (Definition) TableName() string { return "flow_definitions" }

type Instance struct {
	ID                string    `gorm:"primaryKey;size:64"`
	TenantID          string    `gorm:"size:64;index"`
	DefinitionKey     string    `gorm:"size:128;index"`
	DefinitionVersion int
	BusinessKey       string    `gorm:"size:128"`
	BusinessType      string    `gorm:"size:64"`
	StarterID         string    `gorm:"size:64"`
	Status            string    `gorm:"size:32;index"`
	Variables         flow.Vars `gorm:"serializer:json"`
	ParentInstanceID  string    `gorm:"size:64;index"`
	ParentTokenID     string    `gorm:"size:64"`
	CreatedAt         time.Time
	EndedAt           *time.Time
}

func (Instance) TableName() string { return "flow_instances" }

type Token struct {
	ID          string     `gorm:"primaryKey;size:64"`
	InstanceID  string     `gorm:"size:64;index:ix_token_inst"`
	NodeID      string     `gorm:"size:128"`
	Status      string     `gorm:"size:32;index:ix_token_due,priority:1"`
	ParentID    string     `gorm:"size:64"`
	ChildInstID string     `gorm:"size:64"`
	DueAt       *time.Time `gorm:"index:ix_token_due,priority:2"`
	Locals      flow.Vars  `gorm:"serializer:json"`
	CreatedAt   time.Time  `gorm:"index:ix_token_inst,priority:2"`
}

func (Token) TableName() string { return "flow_tokens" }

type Task struct {
	ID         string     `gorm:"primaryKey;size:64"`
	InstanceID string     `gorm:"size:64;index:ix_task_inst"`
	TenantID   string     `gorm:"size:64"`
	NodeID     string     `gorm:"size:128"`
	TokenID    string     `gorm:"size:64;index:ix_task_token"`
	Name       string     `gorm:"size:255"`
	Assignee   string     `gorm:"size:64;index:ix_task_assignee,priority:1"`
	Status     string     `gorm:"size:32;index:ix_task_assignee,priority:2;index:ix_task_due,priority:1"`
	Outcome    string     `gorm:"size:32"`
	Comment    string     `gorm:"type:text"`
	DueAt      *time.Time `gorm:"index:ix_task_due,priority:2"`
	CreatedAt  time.Time
	EndedAt    *time.Time
}

func (Task) TableName() string { return "flow_tasks" }

type HistoryEvent struct {
	ID         string    `gorm:"primaryKey;size:64"`
	InstanceID string    `gorm:"size:64;index:ix_hist_inst,priority:1"`
	Type       string    `gorm:"size:64"`
	NodeID     string    `gorm:"size:128"`
	TaskID     string    `gorm:"size:64"`
	ActorID    string    `gorm:"size:64"`
	Payload    flow.Vars `gorm:"serializer:json"`
	CreatedAt  time.Time `gorm:"index:ix_hist_inst,priority:2"`
}

func (HistoryEvent) TableName() string { return "flow_history_events" }
