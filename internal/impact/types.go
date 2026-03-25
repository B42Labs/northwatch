package impact

// RefType classifies the relationship between a parent and child in the impact tree.
type RefType string

const (
	// RefStrong indicates a strong reference: the child is owned by the parent
	// and will be cascade-deleted by OVSDB when the parent is removed.
	RefStrong RefType = "strong"

	// RefWeak indicates a weak reference: the parent holds a reference to the child
	// that will become dangling if the child is deleted.
	RefWeak RefType = "weak"

	// RefReverse indicates a reverse reference: the child references the parent,
	// meaning the child will lose its reference when the parent is deleted.
	RefReverse RefType = "reverse"

	// RefCorrelation indicates a southbound entity that realizes this northbound
	// entity. It is informational; OVN manages the SB lifecycle automatically.
	RefCorrelation RefType = "correlation"

	// RefRoot is used only for the root node of the impact tree.
	RefRoot RefType = "root"
)

// RefEdge describes a forward reference from a source table to a target table.
type RefEdge struct {
	Column      string  // OVSDB column name on the source (e.g. "ports")
	TargetTable string  // target table (e.g. "Logical_Switch_Port")
	Type        RefType // strong or weak
}

// ReverseRefEdge describes an entity that references the target table.
// Used to find objects that will lose a reference when the target is deleted.
type ReverseRefEdge struct {
	SourceTable string // table that holds the reference (e.g. "Port_Group")
	Column      string // column on SourceTable that contains UUIDs of the target
}

// SBCorrelationDef describes how an NB entity maps to SB entities.
type SBCorrelationDef struct {
	DatapathKey string // ExternalIDs key in Datapath_Binding (e.g. "logical-switch")
}

// ImpactNode is a single node in the computed impact tree.
type ImpactNode struct {
	Database string       `json:"database"`
	Table    string       `json:"table"`
	UUID     string       `json:"uuid"`
	Name     string       `json:"name,omitempty"`
	RefType  RefType      `json:"ref_type"`
	Column   string       `json:"column,omitempty"`
	Children []ImpactNode `json:"children,omitempty"`
}

// ImpactSummary provides aggregate counts for display.
type ImpactSummary struct {
	TotalAffected int            `json:"total_affected"`
	ByTable       map[string]int `json:"by_table"`
	ByRefType     map[string]int `json:"by_ref_type"`
	MaxDepth      int            `json:"max_depth"`
	Truncated     bool           `json:"truncated"`
}

// ImpactResult is the full impact analysis response.
type ImpactResult struct {
	Root    ImpactNode    `json:"root"`
	Summary ImpactSummary `json:"summary"`
}
