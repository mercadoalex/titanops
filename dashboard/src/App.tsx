import { useState } from 'react';
import { ModuleHealth } from './components/ModuleHealth';
import { ActionsFeed } from './components/ActionsFeed';
import { ExplainView } from './components/ExplainView';
import { CorrelationTimeline } from './components/CorrelationTimeline';
import { AuditTrail } from './components/AuditTrail';

type View = 'health' | 'actions' | 'correlations' | 'audit';

export function App() {
  const [view, setView] = useState<View>('health');
  const [selectedActionId, setSelectedActionId] = useState<string | null>(null);

  return (
    <div className="app">
      <header className="app-header">
        <h1>TitanOps</h1>
        <nav aria-label="Main navigation">
          <ul className="nav-list" role="list">
            <li>
              <button
                type="button"
                className={view === 'health' ? 'nav-active' : ''}
                onClick={() => setView('health')}
                aria-current={view === 'health' ? 'page' : undefined}
              >
                Health
              </button>
            </li>
            <li>
              <button
                type="button"
                className={view === 'actions' ? 'nav-active' : ''}
                onClick={() => setView('actions')}
                aria-current={view === 'actions' ? 'page' : undefined}
              >
                Actions
              </button>
            </li>
            <li>
              <button
                type="button"
                className={view === 'correlations' ? 'nav-active' : ''}
                onClick={() => setView('correlations')}
                aria-current={view === 'correlations' ? 'page' : undefined}
              >
                Correlations
              </button>
            </li>
            <li>
              <button
                type="button"
                className={view === 'audit' ? 'nav-active' : ''}
                onClick={() => setView('audit')}
                aria-current={view === 'audit' ? 'page' : undefined}
              >
                Audit
              </button>
            </li>
          </ul>
        </nav>
      </header>

      <main className="app-main">
        {view === 'health' && <ModuleHealth />}
        {view === 'actions' && (
          <div className="actions-layout">
            <ActionsFeed onSelectAction={setSelectedActionId} />
            <ExplainView
              actionId={selectedActionId}
              onClose={() => setSelectedActionId(null)}
            />
          </div>
        )}
        {view === 'correlations' && <CorrelationTimeline />}
        {view === 'audit' && <AuditTrail />}
      </main>
    </div>
  );
}
