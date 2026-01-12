package parser

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"strings"

	"github.com/tenntenn/exp/backend/model"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// BuildSSA builds SSA form from parsed AST
func BuildSSA(fset *token.FileSet, file *ast.File) ([]*model.SSAFunction, []model.ParseError) {
	var errors []model.ParseError

	// Type-check the package
	conf := types.Config{
		Importer: importer.Default(),
		Error: func(err error) {
			errors = append(errors, model.ParseError{
				Message:  err.Error(),
				Position: model.Position{Line: 1, Column: 1, Offset: 0},
				Severity: "warning",
			})
		},
	}

	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
	}

	pkg, err := conf.Check("main", fset, []*ast.File{file}, info)
	if err != nil {
		// Type errors are already collected via conf.Error
		// Continue with SSA construction if possible
		if pkg == nil {
			return nil, errors
		}
	}

	// Build SSA
	prog := ssa.NewProgram(fset, ssa.SanityCheckFunctions)
	ssaPkg := prog.CreatePackage(pkg, []*ast.File{file}, info, true)
	ssaPkg.Build()

	// Convert SSA functions to our model
	var functions []*model.SSAFunction
	for _, mem := range ssaPkg.Members {
		if fn, ok := mem.(*ssa.Function); ok {
			ssaFunc := convertSSAFunction(fset, fn)
			if ssaFunc != nil {
				functions = append(functions, ssaFunc)
			}
		}
	}

	// Also include anonymous functions
	allFuncs := ssautil.AllFunctions(prog)
	for fn := range allFuncs {
		if fn.Pkg == ssaPkg && fn.Parent() != nil {
			ssaFunc := convertSSAFunction(fset, fn)
			if ssaFunc != nil {
				functions = append(functions, ssaFunc)
			}
		}
	}

	return functions, errors
}

// convertSSAFunction converts an ssa.Function to our model
func convertSSAFunction(fset *token.FileSet, fn *ssa.Function) *model.SSAFunction {
	if fn == nil {
		return nil
	}

	result := &model.SSAFunction{
		Name:    fn.Name(),
		Package: fn.Pkg.Pkg.Name(),
	}

	// Get source location
	if fn.Pos().IsValid() {
		pos := fset.Position(fn.Pos())
		result.Location = fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)
	}

	// Build block index map
	blockIndex := make(map[*ssa.BasicBlock]int)
	for i, block := range fn.Blocks {
		blockIndex[block] = i
	}

	// Convert basic blocks
	for i, block := range fn.Blocks {
		bb := model.BasicBlock{
			Index:        i,
			Instructions: []int{},
			Successors:   []int{},
			Predecessors: []int{},
		}

		// Collect predecessor indices
		for _, pred := range block.Preds {
			if idx, ok := blockIndex[pred]; ok {
				bb.Predecessors = append(bb.Predecessors, idx)
			}
		}

		// Collect successor indices
		for _, succ := range block.Succs {
			if idx, ok := blockIndex[succ]; ok {
				bb.Successors = append(bb.Successors, idx)
			}
		}

		result.Blocks = append(result.Blocks, bb)
	}

	// Convert instructions
	instrIndex := 0
	for blockIdx, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			inst := convertInstruction(fset, instr, instrIndex, blockIdx)
			result.Instructions = append(result.Instructions, inst)

			// Add instruction index to block
			result.Blocks[blockIdx].Instructions = append(
				result.Blocks[blockIdx].Instructions,
				instrIndex,
			)

			instrIndex++
		}
	}

	return result
}

// convertInstruction converts an ssa.Instruction to our model
func convertInstruction(fset *token.FileSet, instr ssa.Instruction, index, blockIdx int) model.Instruction {
	inst := model.Instruction{
		Index: index,
		Text:  instr.String(),
		Block: blockIdx,
	}

	// Extract opcode (first word of instruction string)
	text := instr.String()
	if idx := strings.Index(text, " "); idx > 0 {
		// Handle assignment: "t0 = alloc ..." -> opcode is "alloc"
		if strings.Contains(text[:idx], "=") {
			parts := strings.Fields(text)
			if len(parts) >= 3 {
				inst.Opcode = parts[2]
			}
		} else {
			inst.Opcode = text[:idx]
		}
	} else {
		inst.Opcode = text
	}

	// Get source position if available
	if pos := instr.Pos(); pos.IsValid() {
		position := fset.Position(pos)
		inst.Position = model.Position{
			Line:   position.Line,
			Column: position.Column,
			Offset: position.Offset,
		}
	}

	return inst
}
