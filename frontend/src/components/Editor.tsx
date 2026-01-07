import { useRef, useEffect } from 'react';
import Editor from '@monaco-editor/react';
import { useAppStore } from '../store/app';
import type { editor as MonacoEditor } from 'monaco-editor';

export default function CodeEditor() {
  const { code, setCode, selectedASTNode, selectedSSAInstruction, ssa } = useAppStore();
  const editorRef = useRef<MonacoEditor.IStandaloneCodeEditor | null>(null);
  const decorationsRef = useRef<string[]>([]);

  function handleEditorDidMount(editor: MonacoEditor.IStandaloneCodeEditor) {
    editorRef.current = editor;
  }

  // Highlight code when AST node or SSA instruction is selected
  useEffect(() => {
    if (!editorRef.current) return;

    const editor = editorRef.current;
    let newDecorations: MonacoEditor.IModelDeltaDecoration[] = [];

    // Highlight selected AST node
    if (selectedASTNode) {
      const { start, end } = selectedASTNode;
      newDecorations.push({
        range: {
          startLineNumber: start.line,
          startColumn: start.column,
          endLineNumber: end.line,
          endColumn: end.column,
        },
        options: {
          className: 'ast-highlight',
          inlineClassName: 'ast-highlight-inline',
          isWholeLine: false,
          glyphMarginClassName: 'ast-glyph',
        },
      });

      // Scroll to the highlighted line
      editor.revealLineInCenter(start.line);
    }

    // Highlight selected SSA instruction
    if (selectedSSAInstruction !== null && ssa.length > 0) {
      // Find the instruction in SSA functions
      for (const func of ssa) {
        const instr = func.instructions[selectedSSAInstruction];
        if (instr && instr.position && instr.position.line > 0) {
          newDecorations.push({
            range: {
              startLineNumber: instr.position.line,
              startColumn: instr.position.column,
              endLineNumber: instr.position.line,
              endColumn: instr.position.column + 10, // Approximate width
            },
            options: {
              className: 'ssa-highlight',
              inlineClassName: 'ssa-highlight-inline',
              isWholeLine: true,
            },
          });

          // Scroll to the highlighted line
          editor.revealLineInCenter(instr.position.line);
          break;
        }
      }
    }

    // Apply decorations
    decorationsRef.current = editor.deltaDecorations(
      decorationsRef.current,
      newDecorations
    );
  }, [selectedASTNode, selectedSSAInstruction, ssa]);

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div
        style={{
          padding: '8px 16px',
          background: '#1e1e1e',
          color: '#fff',
          borderBottom: '1px solid #333',
          fontWeight: 'bold',
        }}
      >
        Code Editor
      </div>
      <div style={{ flex: 1 }}>
        <Editor
          height="100%"
          defaultLanguage="go"
          theme="vs-dark"
          value={code}
          onChange={(value) => setCode(value || '')}
          onMount={handleEditorDidMount}
          options={{
            minimap: { enabled: false },
            fontSize: 14,
            lineNumbers: 'on',
            scrollBeyondLastLine: false,
            automaticLayout: true,
          }}
        />
      </div>
      <style>{`
        .ast-highlight {
          background-color: rgba(78, 201, 176, 0.15);
        }
        .ast-highlight-inline {
          background-color: rgba(78, 201, 176, 0.2);
        }
        .ssa-highlight {
          background-color: rgba(206, 145, 120, 0.15);
        }
        .ssa-highlight-inline {
          background-color: rgba(206, 145, 120, 0.2);
        }
        .ast-glyph {
          background-color: rgba(78, 201, 176, 0.5);
          width: 3px !important;
          margin-left: 3px;
        }
      `}</style>
    </div>
  );
}
