package model

// ParseRequest represents a request to parse Go code
type ParseRequest struct {
	Code   string `json:"code"`
	Format string `json:"format"` // "single" or "txtar"
}

// Position represents a position in source code
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

// ASTNode represents a node in the AST
type ASTNode struct {
	Type     string      `json:"type"`
	Start    Position    `json:"start"`
	End      Position    `json:"end"`
	Value    interface{} `json:"value,omitempty"`
	Children []*ASTNode  `json:"children,omitempty"`
}

// SSAFunction represents a function in SSA form
type SSAFunction struct {
	Name         string        `json:"name"`
	Package      string        `json:"package"`
	Location     string        `json:"location"`
	Instructions []Instruction `json:"instructions"`
	Blocks       []BasicBlock  `json:"blocks"`
}

// Instruction represents a single SSA instruction
type Instruction struct {
	Index    int      `json:"index"`
	Text     string   `json:"text"`
	Opcode   string   `json:"opcode"`
	Position Position `json:"position"`
	Block    int      `json:"block"`
}

// BasicBlock represents a basic block in SSA
type BasicBlock struct {
	Index        int   `json:"index"`
	Instructions []int `json:"instructions"` // indices into Instructions array
	Successors   []int `json:"successors"`   // indices of successor blocks
	Predecessors []int `json:"predecessors"` // indices of predecessor blocks
}

// ParseResponse represents the response from parsing
type ParseResponse struct {
	AST    *ASTNode       `json:"ast,omitempty"`
	SSA    []*SSAFunction `json:"ssa,omitempty"`
	Errors []ParseError   `json:"errors,omitempty"`
}

// ParseError represents a parse or type-checking error
type ParseError struct {
	Message  string   `json:"message"`
	Position Position `json:"position"`
	Severity string   `json:"severity"` // "error" or "warning"
}
