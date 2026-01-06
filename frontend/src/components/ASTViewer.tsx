import { useState } from 'react';
import { useAppStore } from '../store/app';
import type { ASTNode } from '../types';

export default function ASTViewer() {
  const { ast, setSelectedASTNode } = useAppStore();

  if (!ast) {
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
          AST Viewer
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
          Click "Parse" to visualize AST
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
        AST Viewer
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
        <ASTNodeTree node={ast} onNodeClick={setSelectedASTNode} />
      </div>
    </div>
  );
}

interface ASTNodeTreeProps {
  node: ASTNode;
  depth?: number;
  onNodeClick: (node: ASTNode) => void;
}

function ASTNodeTree({ node, depth = 0, onNodeClick }: ASTNodeTreeProps) {
  const [isExpanded, setIsExpanded] = useState(depth < 2);
  const hasChildren = node.children && node.children.length > 0;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      setIsExpanded(!isExpanded);
    }
    onNodeClick(node);
  };

  return (
    <div style={{ marginLeft: depth * 16 }}>
      <div
        onClick={handleClick}
        style={{
          padding: '2px 4px',
          cursor: 'pointer',
          borderRadius: '3px',
          transition: 'background 0.15s',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.background = '#2a2a2a';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'transparent';
        }}
      >
        <span style={{ color: '#888', marginRight: '4px' }}>
          {hasChildren ? (isExpanded ? '▼' : '▶') : '•'}
        </span>
        <span style={{ color: '#4ec9b0' }}>{node.type}</span>
        {node.value && (
          <span style={{ color: '#9cdcfe', marginLeft: '8px' }}>
            {JSON.stringify(node.value).slice(0, 50)}
          </span>
        )}
        <span style={{ color: '#666', marginLeft: '8px', fontSize: '11px' }}>
          {node.start.line}:{node.start.column}
        </span>
      </div>
      {isExpanded && hasChildren && (
        <div>
          {node.children!.map((child, index) => (
            <ASTNodeTree
              key={index}
              node={child}
              depth={depth + 1}
              onNodeClick={onNodeClick}
            />
          ))}
        </div>
      )}
    </div>
  );
}
