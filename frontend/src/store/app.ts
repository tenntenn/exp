import { create } from 'zustand';
import type { ASTNode, SSAFunction, ParseError } from '../types';

interface AppState {
  code: string;
  ast: ASTNode | null;
  ssa: SSAFunction[];
  errors: ParseError[];
  selectedASTNode: ASTNode | null;
  selectedSSAInstruction: number | null;
  isLoading: boolean;

  setCode: (code: string) => void;
  setAST: (ast: ASTNode | null) => void;
  setSSA: (ssa: SSAFunction[]) => void;
  setErrors: (errors: ParseError[]) => void;
  setSelectedASTNode: (node: ASTNode | null) => void;
  setSelectedSSAInstruction: (index: number | null) => void;
  setIsLoading: (isLoading: boolean) => void;
  parse: () => Promise<void>;
  share: () => Promise<string>;
  loadFromHash: (hash: string) => Promise<void>;
}

export const useAppStore = create<AppState>((set, get) => ({
  code: `package main

import "fmt"

func main() {
\tfmt.Println("Hello, World!")
}`,
  ast: null,
  ssa: [],
  errors: [],
  selectedASTNode: null,
  selectedSSAInstruction: null,
  isLoading: false,

  setCode: (code) => set({ code }),
  setAST: (ast) => set({ ast }),
  setSSA: (ssa) => set({ ssa }),
  setErrors: (errors) => set({ errors }),
  setSelectedASTNode: (node) => set({ selectedASTNode: node }),
  setSelectedSSAInstruction: (index) => set({ selectedSSAInstruction: index }),
  setIsLoading: (isLoading) => set({ isLoading }),

  parse: async () => {
    const { code } = get();
    set({ isLoading: true });

    try {
      // Use Connect RPC with JSON-based communication
      const response = await fetch('/parser.v1.ParserService/Parse', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          code,
          format: 'single',
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      set({
        ast: data.ast || null,
        ssa: data.ssa || [],
        errors: data.errors || [],
        isLoading: false,
      });
    } catch (error) {
      console.error('Parse error:', error);
      set({
        errors: [
          {
            message: error instanceof Error ? error.message : 'Unknown error',
            position: { line: 1, column: 1, offset: 0 },
            severity: 'error',
          },
        ],
        isLoading: false,
      });
    }
  },

  share: async () => {
    const { code } = get();
    set({ isLoading: true });

    try {
      const response = await fetch('/parser.v1.ParserService/Share', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          code,
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      set({ isLoading: false });

      // Update URL with hash parameter
      const hash = data.hash;
      window.history.pushState({}, '', `?share=${hash}`);

      return hash;
    } catch (error) {
      console.error('Share error:', error);
      set({ isLoading: false });
      throw error;
    }
  },

  loadFromHash: async (hash: string) => {
    set({ isLoading: true });

    try {
      const response = await fetch('/parser.v1.ParserService/Load', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          hash,
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      set({
        code: data.code,
        isLoading: false
      });

      // Auto-parse after loading
      await get().parse();
    } catch (error) {
      console.error('Load error:', error);
      set({
        errors: [
          {
            message: error instanceof Error ? error.message : 'Failed to load code',
            position: { line: 1, column: 1, offset: 0 },
            severity: 'error',
          },
        ],
        isLoading: false,
      });
    }
  },
}));
