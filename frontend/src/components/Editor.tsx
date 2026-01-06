import Editor from '@monaco-editor/react';
import { useAppStore } from '../store/app';

export default function CodeEditor() {
  const { code, setCode } = useAppStore();

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
          options={{
            minimap: { enabled: false },
            fontSize: 14,
            lineNumbers: 'on',
            scrollBeyondLastLine: false,
            automaticLayout: true,
          }}
        />
      </div>
    </div>
  );
}
