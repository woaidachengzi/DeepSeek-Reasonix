package event

// ComputerUsePreview is a bounded, display-only image from an authorized
// observation. It is excluded from the durable turn ledger.
type ComputerUsePreview struct {
	AppName  string `json:"appName"`
	BundleID string `json:"bundleId"`
	PID      int    `json:"pid"`
	WindowID int    `json:"windowId"`
	Action   string `json:"action"`
	Image    string `json:"image"`
}

// ShellExecution mirrors tool.ShellExecution for event sinks without importing
// the tool package (event is a lower-level dependency of tool consumers).
type ShellExecution struct {
	Kind           string `json:"kind,omitempty"`
	Shell          string `json:"shell,omitempty"`
	ShellVersion   string `json:"shellVersion,omitempty"`
	Platform       string `json:"platform,omitempty"`
	SupportsAndAnd bool   `json:"supportsAndAnd"`
	State          string `json:"state,omitempty"`
	FailurePhase   string `json:"failurePhase,omitempty"`
	ExitCode       *int   `json:"exitCode,omitempty"`
	OutputTail     string `json:"outputTail,omitempty"`
	MutationRisk   string `json:"mutationRisk,omitempty"`
	Verification   string `json:"verification,omitempty"`
	DurationMs     int64  `json:"durationMs,omitempty"`
}
