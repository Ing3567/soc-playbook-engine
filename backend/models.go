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

type EdgeCondition struct {
	Field string `json:"field" bson:"field"`
	Op    string `json:"op" bson:"op"`
	Value any    `json:"value,omitempty" bson:"value,omitempty"`
}

type Edge struct {
	ID        string         `json:"id" bson:"id"`
	Source    string         `json:"source" bson:"source"`
	Target    string         `json:"target" bson:"target"`
	Label     string         `json:"label,omitempty" bson:"label,omitempty"`
	Condition *EdgeCondition `json:"condition,omitempty" bson:"condition,omitempty"`
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

type ScenarioGroundTruth struct {
	ExpectedVerdict string   `json:"expected_verdict" bson:"expected_verdict"`
	RequiredNodes   []string `json:"required_nodes,omitempty" bson:"required_nodes,omitempty"`
	ExpectedEdges   []string `json:"expected_edges,omitempty" bson:"expected_edges,omitempty"`
	MaxDurationSec  int      `json:"max_duration_sec,omitempty" bson:"max_duration_sec,omitempty"`
}

type Scenario struct {
	ID          string                               `json:"id" bson:"_id"`
	Title       string                               `json:"title" bson:"title"`
	Description string                               `json:"description" bson:"description"`
	Difficulty  string                               `json:"difficulty" bson:"difficulty"`
	PlaybookID  string                               `json:"playbook_id,omitempty" bson:"playbook_id,omitempty"`
	Alert       map[string]any                       `json:"alert" bson:"alert"`
	ToolMocks   map[string]map[string]map[string]any `json:"tool_mocks,omitempty" bson:"tool_mocks,omitempty"`
	GroundTruth *ScenarioGroundTruth                 `json:"ground_truth,omitempty" bson:"ground_truth,omitempty"`
	IsBuiltin   bool                                 `json:"is_builtin" bson:"is_builtin"`
	CreatedAt   time.Time                            `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time                            `json:"updated_at" bson:"updated_at"`
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
	RuntimeAuth    string            `json:"runtime_auth,omitempty"`
	ExecutionMode  string            `json:"execution_mode,omitempty"`
	ScenarioID     string            `json:"scenario_id,omitempty"`
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

type ToolAuth struct {
	Type         string `json:"type" bson:"type"`
	In           string `json:"in,omitempty" bson:"in,omitempty"`
	KeyName      string `json:"key_name,omitempty" bson:"key_name,omitempty"`
	Prefix       string `json:"prefix,omitempty" bson:"prefix,omitempty"`
	EnvVar       string `json:"env_var,omitempty" bson:"env_var,omitempty"`
	AllowRuntime bool   `json:"allow_runtime,omitempty" bson:"allow_runtime,omitempty"`
}

type ToolParameter struct {
	Key          string `json:"key" bson:"key"`
	Label        string `json:"label,omitempty" bson:"label,omitempty"`
	In           string `json:"in" bson:"in"`
	Mode         string `json:"mode" bson:"mode"`
	Value        string `json:"value,omitempty" bson:"value,omitempty"`
	DefaultValue string `json:"default_value,omitempty" bson:"default_value,omitempty"`
	Required     bool   `json:"required,omitempty" bson:"required,omitempty"`
	IsSecret     bool   `json:"is_secret,omitempty" bson:"is_secret,omitempty"`
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
	Auth            *ToolAuth       `json:"auth,omitempty" bson:"auth,omitempty"`
	AuthConfigured  bool            `json:"auth_configured" bson:"-"`
	Params          []ToolParameter `json:"params,omitempty" bson:"params,omitempty"`
	PayloadTemplate map[string]any  `json:"payload_template,omitempty" bson:"payload_template,omitempty"`
	MockOutput      map[string]any  `json:"mock_output,omitempty" bson:"mock_output,omitempty"`
	IsBuiltin       bool            `json:"is_builtin" bson:"is_builtin"`
	Enabled         bool            `json:"enabled" bson:"enabled"`
	CreatedAt       time.Time       `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" bson:"updated_at"`
}

type UserDocument struct {
	ID           string    `json:"id" bson:"_id"`
	Username     string    `json:"username" bson:"username"`
	DisplayName  string    `json:"display_name" bson:"display_name"`
	Role         string    `json:"role" bson:"role"`
	PasswordHash string    `json:"-" bson:"password_hash"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}

type AuthClaims struct {
	UserID      string `json:"sub"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Exp         int64  `json:"exp"`
}

type RunScore struct {
	Total            int      `json:"total" bson:"total"`
	VerdictPoints    int      `json:"verdict_points" bson:"verdict_points"`
	CoveragePoints   int      `json:"coverage_points" bson:"coverage_points"`
	PathPoints       int      `json:"path_points" bson:"path_points"`
	SpeedPoints      int      `json:"speed_points" bson:"speed_points"`
	DurationSec      int      `json:"duration_sec" bson:"duration_sec"`
	MaxDurationSec   int      `json:"max_duration_sec,omitempty" bson:"max_duration_sec,omitempty"`
	ExpectedVerdict  string   `json:"expected_verdict,omitempty" bson:"expected_verdict,omitempty"`
	SubmittedVerdict string   `json:"submitted_verdict" bson:"submitted_verdict"`
	MissedNodes      []string `json:"missed_nodes,omitempty" bson:"missed_nodes,omitempty"`
	MissedEdges      []string `json:"missed_edges,omitempty" bson:"missed_edges,omitempty"`
	Feedback         []string `json:"feedback" bson:"feedback"`
}

type SimulationRun struct {
	ID             string                    `json:"id" bson:"_id"`
	ScenarioID     string                    `json:"scenario_id,omitempty" bson:"scenario_id,omitempty"`
	ScenarioTitle  string                    `json:"scenario_title,omitempty" bson:"scenario_title,omitempty"`
	PlaybookID     string                    `json:"playbook_id,omitempty" bson:"playbook_id,omitempty"`
	PlaybookTitle  string                    `json:"playbook_title" bson:"playbook_title"`
	Nodes          []Node                    `json:"nodes" bson:"nodes"`
	Edges          []Edge                    `json:"edges" bson:"edges"`
	Alert          map[string]any            `json:"alert,omitempty" bson:"alert,omitempty"`
	TraineeID      string                    `json:"trainee_id" bson:"trainee_id"`
	TraineeName    string                    `json:"trainee_name" bson:"trainee_name"`
	Mode           string                    `json:"mode" bson:"mode"`
	Status         string                    `json:"status" bson:"status"`
	NodeStatuses   map[string]string         `json:"node_statuses" bson:"node_statuses"`
	NodeOutputs    map[string]map[string]any `json:"node_outputs" bson:"node_outputs"`
	TraversedEdges []string                  `json:"traversed_edges,omitempty" bson:"traversed_edges,omitempty"`
	FinalVerdict   string                    `json:"final_verdict,omitempty" bson:"final_verdict,omitempty"`
	Summary        string                    `json:"summary,omitempty" bson:"summary,omitempty"`
	Score          *RunScore                 `json:"score,omitempty" bson:"score,omitempty"`
	StartedAt      time.Time                 `json:"started_at" bson:"started_at"`
	CompletedAt    *time.Time                `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	UpdatedAt      time.Time                 `json:"updated_at" bson:"updated_at"`
}

type RunEvent struct {
	ID         string         `json:"id" bson:"_id"`
	RunID      string         `json:"run_id" bson:"run_id"`
	Seq        int            `json:"seq" bson:"seq"`
	NodeID     string         `json:"node_id,omitempty" bson:"node_id,omitempty"`
	NodeLabel  string         `json:"node_label,omitempty" bson:"node_label,omitempty"`
	Action     string         `json:"action" bson:"action"`
	Status     string         `json:"status,omitempty" bson:"status,omitempty"`
	Input      map[string]any `json:"input,omitempty" bson:"input,omitempty"`
	Output     map[string]any `json:"output,omitempty" bson:"output,omitempty"`
	Verdict    string         `json:"verdict,omitempty" bson:"verdict,omitempty"`
	Notes      string         `json:"notes,omitempty" bson:"notes,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty" bson:"duration_ms,omitempty"`
	Timestamp  time.Time      `json:"timestamp" bson:"timestamp"`
}

type StartRunRequest struct {
	PlaybookID    string         `json:"playbook_id,omitempty"`
	PlaybookTitle string         `json:"playbook_title,omitempty"`
	ScenarioID    string         `json:"scenario_id,omitempty"`
	Nodes         []Node         `json:"nodes,omitempty"`
	Edges         []Edge         `json:"edges,omitempty"`
	Alert         map[string]any `json:"alert,omitempty"`
	Mode          string         `json:"mode,omitempty"`
}

type RunStepRequest struct {
	RuntimeInputs map[string]string `json:"runtime_inputs,omitempty"`
	RuntimeAuth   string            `json:"runtime_auth,omitempty"`
	ManualVerdict string            `json:"manual_verdict,omitempty"`
	Notes         string            `json:"notes,omitempty"`
}

type CompleteRunRequest struct {
	FinalVerdict string `json:"final_verdict"`
	Summary      string `json:"summary,omitempty"`
}
