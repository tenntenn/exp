import Header from './components/Header';
import CodeEditor from './components/Editor';
import ASTViewer from './components/ASTViewer';
import SSAViewer from './components/SSAViewer';

function App() {
  return (
    <div
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      <Header />
      <div
        style={{
          flex: 1,
          display: 'flex',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            flex: 1,
            borderRight: '1px solid #333',
            minWidth: 0,
          }}
        >
          <CodeEditor />
        </div>
        <div
          style={{
            flex: 1,
            borderRight: '1px solid #333',
            minWidth: 0,
          }}
        >
          <ASTViewer />
        </div>
        <div
          style={{
            flex: 1,
            minWidth: 0,
          }}
        >
          <SSAViewer />
        </div>
      </div>
    </div>
  );
}

export default App;
