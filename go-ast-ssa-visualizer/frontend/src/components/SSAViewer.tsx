import { useState } from 'react';
import { useAppStore } from '../store/app';
import type { SSAFunction, Instruction } from '../types';

export default function SSAViewer() {
  const { ssa, selectedSSAInstruction, setSelectedSSAInstruction } = useAppStore();

  if (!ssa || ssa.length === 0) {
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
          SSA Viewer
        </div>
        <div
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#888',
            background: '#252526',
          }}
        >
          Click "Parse" to visualize SSA
        </div>
      </div>
    );
  }

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
        SSA Viewer
      </div>
      <div
        style={{
          flex: 1,
          overflow: 'auto',
          background: '#252526',
          color: '#ccc',
          padding: '8px',
          fontFamily: 'monospace',
          fontSize: '13px',
        }}
      >
        {ssa.map((func, index) => (
          <SSAFunctionView
            key={index}
            func={func}
            selectedInstruction={selectedSSAInstruction}
            onInstructionClick={setSelectedSSAInstruction}
          />
        ))}
      </div>
    </div>
  );
}

interface SSAFunctionViewProps {
  func: SSAFunction;
  selectedInstruction: number | null;
  onInstructionClick: (index: number) => void;
}

function SSAFunctionView({ func, selectedInstruction, onInstructionClick }: SSAFunctionViewProps) {
  const [isExpanded, setIsExpanded] = useState(true);

  return (
    <div style={{ marginBottom: '16px' }}>
      <div
        onClick={() => setIsExpanded(!isExpanded)}
        style={{
          padding: '4px 8px',
          background: '#1e1e1e',
          borderRadius: '4px',
          cursor: 'pointer',
          marginBottom: '8px',
        }}
      >
        <span style={{ color: '#888', marginRight: '8px' }}>
          {isExpanded ? '▼' : '▶'}
        </span>
        <span style={{ color: '#dcdcaa', fontWeight: 'bold' }}>
          {func.name}
        </span>
        {func.location && (
          <span style={{ color: '#666', marginLeft: '8px', fontSize: '11px' }}>
            {func.location}
          </span>
        )}
      </div>

      {isExpanded && (
        <div style={{ marginLeft: '16px' }}>
          {func.blocks.map((block) => (
            <div key={block.index} style={{ marginBottom: '12px' }}>
              <div
                style={{
                  color: '#569cd6',
                  marginBottom: '4px',
                  fontWeight: 'bold',
                }}
              >
                block {block.index}:
                {block.predecessors.length > 0 && (
                  <span style={{ color: '#888', marginLeft: '8px' }}>
                    ← {block.predecessors.join(', ')}
                  </span>
                )}
              </div>
              {block.instructions.map((instrIndex) => {
                const instr = func.instructions[instrIndex];
                const isSelected = selectedInstruction === instrIndex;

                return (
                  <InstructionLine
                    key={instrIndex}
                    instrIndex={instrIndex}
                    instr={instr}
                    isSelected={isSelected}
                    onClick={() => onInstructionClick(instrIndex)}
                  />
                );
              })}
              {block.successors.length > 0 && (
                <div
                  style={{
                    color: '#888',
                    marginLeft: '16px',
                    marginTop: '4px',
                    fontSize: '12px',
                  }}
                >
                  → {block.successors.join(', ')}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

interface InstructionLineProps {
  instrIndex: number;
  instr: Instruction;
  isSelected: boolean;
  onClick: () => void;
}

function InstructionLine({ instrIndex, instr, isSelected, onClick }: InstructionLineProps) {
  const [isHovered, setIsHovered] = useState(false);

  const getBackgroundColor = () => {
    if (isSelected) return '#264f78';
    if (isHovered) return '#2a2a2a';
    return 'transparent';
  };

  return (
    <div
      onClick={onClick}
      style={{
        padding: '2px 8px',
        cursor: 'pointer',
        borderRadius: '3px',
        transition: 'background 0.15s',
        marginLeft: '16px',
        background: getBackgroundColor(),
      }}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      <span style={{ color: '#666', marginRight: '8px' }}>
        {instrIndex.toString().padStart(3, ' ')}:
      </span>
      <span style={{ color: '#ce9178' }}>{instr.text}</span>
      {instr.position && instr.position.line > 0 && (
        <span
          style={{
            color: '#666',
            marginLeft: '8px',
            fontSize: '11px',
          }}
        >
          @{instr.position.line}:{instr.position.column}
        </span>
      )}
    </div>
  );
}
