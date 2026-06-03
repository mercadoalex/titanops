import { useEffect, useState } from 'react';
import { fetchHealth } from '../api/client';
import type { ModuleHealth as ModuleHealthType } from '../types';

const STATUS_LABELS: Record<string, string> = {
  operational: 'Operational',
  degraded: 'Degraded',
  unavailable: 'Unavailable',
};

function statusClass(status: string): string {
  switch (status) {
    case 'operational':
      return 'status-operational';
    case 'degraded':
      return 'status-degraded';
    case 'unavailable':
      return 'status-unavailable';
    default:
      return '';
  }
}

export function ModuleHealth() {
  const [modules, setModules] = useState<ModuleHealthType[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    async function poll() {
      try {
        const data = await fetchHealth();
        if (active) {
          setModules(data);
          setError(null);
        }
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'Failed to fetch health');
      }
    }

    poll();
    const interval = setInterval(poll, 5000); // refresh every 5s
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, []);

  return (
    <section aria-labelledby="module-health-heading">
      <h2 id="module-health-heading">Module Health</h2>
      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}
      <ul className="module-health-list" role="list">
        {modules.map((m) => (
          <li key={m.module} className={`module-health-item ${statusClass(m.status)}`}>
            <span
              className="status-indicator"
              aria-label={`${m.module} status: ${STATUS_LABELS[m.status] ?? m.status}`}
            />
            <span className="module-name">{m.module}</span>
            <span className="module-status">{STATUS_LABELS[m.status] ?? m.status}</span>
            <time className="module-since" dateTime={m.since}>
              since {new Date(m.since).toLocaleString()}
            </time>
          </li>
        ))}
      </ul>
    </section>
  );
}
