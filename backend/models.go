package main

import "time"

type Line struct {
	Level int    `json:"level" bson:"level"`
	Text  string `json:"text" bson:"text"`
}

type Branch struct {
	Label string `json:"label" bson:"label"`
	Steps []Step `json:"steps" bson:"steps"`
}

type Step struct {
	Type     string   `json:"type" bson:"type"`
	Label    string   `json:"label" bson:"label"`
	Branches []Branch `json:"branches,omitempty" bson:"branches,omitempty"`
}

type Node struct {
	ID    string `json:"id" bson:"id"`
	Type  string `json:"type" bson:"type"`
	Label string `json:"label" bson:"label"`
	Tool  string `json:"tool" bson:"tool"`
}

type Edge struct {
	ID     string `json:"id" bson:"id"`
	Source string `json:"source" bson:"source"`
	Target string `json:"target" bson:"target"`
	Label  string `json:"label,omitempty" bson:"label,omitempty"`
}

type PlaybookDocument struct {
	ID        string    `json:"id" bson:"_id"`
	Title     string    `json:"title" bson:"title"`
	RawText   string    `json:"raw_text" bson:"raw_text"`
	Engine    string    `json:"engine" bson:"engine"`
	Nodes     []Node    `json:"nodes" bson:"nodes"`
	Edges     []Edge    `json:"edges" bson:"edges"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

type ExecutionRequest struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Tool   string `json:"tool"`
}

type ExecutionResult struct {
	NodeID     string         `json:"node_id"`
	Status     string         `json:"status"`
	DurationMs int64          `json:"duration_ms"`
	Timestamp  string         `json:"timestamp"`
	Output     map[string]any `json:"output"`
}

type ExportRequest struct {
	Title string `json:"title"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}