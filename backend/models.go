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
	Tool     string   `json:"tool,omitempty" bson:"tool,omitempty"`
	Branches []Branch `json:"branches,omitempty" bson:"branches,omitempty"`
}

type Node struct {
	ID           string            `json:"id" bson:"id"`
	Type         string            `json:"type" bson:"type"`
	Label        string            `json:"label" bson:"label"`
	Tool         string            `json:"tool" bson:"tool"`
	ToolSource   string            `json:"tool_source,omitempty" bson:"tool_source,omitempty"`
	PayloadMode  string            `json:"payload_mode,omitempty" bson:"payload_mode,omitempty"`
	ParamModes   map[string]string `json:"param_modes,omitempty" bson:"param_modes,omitempty"`
	InputPayload map[string]string `json:"input_payload,omitempty" bson:"input_payload,omitempty"`
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
	NodeID         string            `json:"node_id"`
	Type           string            `json:"type"`
	Label          string            `json:"label"`
	Tool           string            `json:"tool"`
	PayloadMode    string            `json:"payload_mode,omitempty"`
	ParamModes     map[string]string `json:"param_modes,omitempty"`
	InputPayload   map[string]string `json:"input_payload,omitempty"`
	RuntimeInputs  map[string]string `json:"runtime_inputs,omitempty"`
	RuntimeContext map[string]any    `json:"runtime_context,omitempty"`
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

type ToolParameter struct {
	Key          string `json:"key" bson:"key"`
	Label        string `json:"label,omitempty" bson:"label,omitempty"`
	In           string `json:"in" bson:"in"`
	Mode         string `json:"mode" bson:"mode"`
	Value        string `json:"value,omitempty" bson:"value,omitempty"`
	DefaultValue string `json:"default_value,omitempty" bson:"default_value,omitempty"`
	Required     bool   `json:"required,omitempty" bson:"required,omitempty"`
}

type ToolDefinition struct {
	ID              string          `json:"id" bson:"_id"`
	Name            string          `json:"name" bson:"name"`
	Category        string          `json:"category" bson:"category"`
	Description     string          `json:"description" bson:"description"`
	Keywords        []string        `json:"keywords" bson:"keywords"`
	CommandType     string          `json:"command_type" bson:"command_type"`
	HTTPMethod      string          `json:"http_method,omitempty" bson:"http_method,omitempty"`
	Endpoint        string          `json:"endpoint,omitempty" bson:"endpoint,omitempty"`
	PayloadMode     string          `json:"payload_mode,omitempty" bson:"payload_mode,omitempty"`
	Params          []ToolParameter `json:"params,omitempty" bson:"params,omitempty"`
	PayloadTemplate map[string]any  `json:"payload_template,omitempty" bson:"payload_template,omitempty"`
	MockOutput      map[string]any  `json:"mock_output,omitempty" bson:"mock_output,omitempty"`
	IsBuiltin       bool            `json:"is_builtin" bson:"is_builtin"`
	Enabled         bool            `json:"enabled" bson:"enabled"`
	CreatedAt       time.Time       `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" bson:"updated_at"`
}
