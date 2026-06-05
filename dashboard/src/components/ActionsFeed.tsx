import { useEffect, useState } from 'react';
import { fetchActions } from '../api/client';
import type { AutonomousAction } from '../types';

interface ActionsFeedProps {
  onSelectAction?: (actionId: string) => void;
}

export function ActionsFeed({ onSelectAction }: ActionsFeedProps) {
  const [actions, setActions] = useState<AutonomousAction[]>([]);
  const [moduleFilter, setModuleFilter] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      try {
        const data = await fetchActions(50);
        if (active) {
          setActions(data);
          setError(null);
        }
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'Failed to fetch actions');
      }
    }

    load();
    const interval = setInterval(load, 10000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, []);

  const filteredActions = moduleFilter
    ? actions.filter((a) => a.module === moduleFilter)
    : actions;

  return (
    <section aria-labelledby="actions-feed-heading">
      <h2 id="actions-feed-heading">Recent Actions</h2>

      <div className="actions-filters" role="group" aria-label="Actions feed filters">
        <label htmlFor="actions-module-filter">
          Module:{' '}
          <select
            id="actions-module-filter"
            value={moduleFilter}
            onChange={(e) => setModuleFilter(e.target.value)}
          >
            <option value="">All</option>
            <option value="earthworm">Earthworm</option>
            <option value="tlapix">Tlapix</option>
            <option value="ebeecontrol">eBeeControl</option>
            <option value="quack">Quack</option>
            <option value="ollinai">OllinAI</option>
          </select>
        </label>
      </div>

      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}
      <ol className="actions-feed-list" role="list">
        {filteredActions.map((action) => (
          <li key={action.id} className="action-item">
            <header className="action-header">
              <strong>{action.module}</strong>
              <span className="action-type">{action.action_type}</span>
              <span className={`action-outcome outcome-${action.outcome}`}>
                {action.outcome}
              </span>
              <time dateTime={action.timestamp}>
                {new Date(action.timestamp).toLocaleString()}
              </time>
            </header>
            <dl className="reasoning-chain">
              <dt>Observation</dt>
              <dd>{action.reasoning.observation}</dd>
              <dt>Analysis</dt>
              <dd>{action.reasoning.analysis}</dd>
              <dt>Action</dt>
              <dd>{action.reasoning.action}</dd>
            </dl>
            <p className="action-confidence">
              Confidence: {(action.confidence * 100).toFixed(1)}%
            </p>
            {onSelectAction && (
              <button
                type="button"
                className="explain-button"
                onClick={() => onSelectAction(action.id)}
                aria-label={`Explain action ${action.id}`}
              >
                Explain
              </button>
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
