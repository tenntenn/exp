import { useState } from 'react';
import { useAppStore } from '../store/app';

export default function Header() {
  const { parse, share, isLoading, errors } = useAppStore();
  const [shareStatus, setShareStatus] = useState('');

  return (
    <div
      style={{
        height: '50px',
        background: '#1e1e1e',
        borderBottom: '1px solid #333',
        display: 'flex',
        alignItems: 'center',
        padding: '0 16px',
        justifyContent: 'space-between',
      }}
    >
      <h1 style={{ color: '#fff', fontSize: '18px', margin: 0 }}>
        Go AST/SSA Visualizer
      </h1>

      <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
        {shareStatus && (
          <div style={{ color: '#4ec9b0', fontSize: '13px' }}>
            {shareStatus}
          </div>
        )}

        {errors.length > 0 && (
          <div style={{ color: '#f48771', fontSize: '13px' }}>
            {errors.length} error{errors.length > 1 ? 's' : ''}
          </div>
        )}

        <button
          onClick={async () => {
            try {
              await share();
              setShareStatus('Link copied!');
              // Copy URL to clipboard
              navigator.clipboard.writeText(window.location.href);
              setTimeout(() => setShareStatus(''), 3000);
            } catch (error) {
              setShareStatus('Failed to share');
              setTimeout(() => setShareStatus(''), 3000);
            }
          }}
          disabled={isLoading}
          style={{
            padding: '8px 20px',
            background: isLoading ? '#555' : '#0e8c39',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: isLoading ? 'not-allowed' : 'pointer',
            fontSize: '14px',
            fontWeight: '500',
            transition: 'background 0.2s',
          }}
          onMouseEnter={(e) => {
            if (!isLoading) {
              e.currentTarget.style.background = '#11a843';
            }
          }}
          onMouseLeave={(e) => {
            if (!isLoading) {
              e.currentTarget.style.background = '#0e8c39';
            }
          }}
        >
          Share
        </button>

        <button
          onClick={parse}
          disabled={isLoading}
          style={{
            padding: '8px 20px',
            background: isLoading ? '#555' : '#0e639c',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: isLoading ? 'not-allowed' : 'pointer',
            fontSize: '14px',
            fontWeight: '500',
            transition: 'background 0.2s',
          }}
          onMouseEnter={(e) => {
            if (!isLoading) {
              e.currentTarget.style.background = '#1177bb';
            }
          }}
          onMouseLeave={(e) => {
            if (!isLoading) {
              e.currentTarget.style.background = '#0e639c';
            }
          }}
        >
          {isLoading ? 'Parsing...' : 'Parse'}
        </button>
      </div>
    </div>
  );
}
