export interface Position {
  line: number;
  column: number;
  offset: number;
}

export interface ASTNode {
  type: string;
  start: Position;
  end: Position;
  value?: Record<string, unknown>;
  children?: ASTNode[];
}

export interface Instruction {
  index: number;
  text: string;
  opcode: string;
  position: Position;
  block: number;
}

export interface BasicBlock {
  index: number;
  instructions: number[];
  successors: number[];
  predecessors: number[];
}

export interface SSAFunction {
  name: string;
  package: string;
  location: string;
  instructions: Instruction[];
  blocks: BasicBlock[];
}

export interface ParseError {
  message: string;
  position: Position;
  severity: 'error' | 'warning';
}

export interface ParseResponse {
  ast?: ASTNode;
  ssa?: SSAFunction[];
  errors?: ParseError[];
}
