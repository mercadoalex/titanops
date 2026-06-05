import { useEffect, useState } from 'react';
import { fetchAudit } from '../api/client';
import type { AuditEntry, AuditFilter } from '../types';

const PAGE_SIZE = 20;

export function AuditTrail() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [filter, setFilter] = useState<AuditFilter>({ page: 1, page_size: PAGE_SIZE });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let active = true;
    setLoading(true);

    fetchAudit(filter)
      .then((data) => {
        if (active) {
          setEntries(data);
          setError(null);
        }
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : 'Failed to fetch audit trail');
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [filter]);

  function handleModuleFilter(module: string) {
    setFilter((prev) => ({ ...prev, module: module || undefined, page: 1 }));
  }

  function handleActionTypeFilter(actionType: string) {
    setFilter((prev) => ({ ...prev, action_type: actionType || undefined, page: 1 }));
  }

  function handleNextPage() {
    setFilter((prev) => ({ ...prev, page: (prev.page ?? 1) + 1 }));
  }

  function handlePrevPage() {
    setFilter((prev) => ({ ...prev, page: Math.max(1, (prev.page ?? 1) - 1) }));
  }

  return (
    <section aria-labelledby="audit-heading">
      <h2 id="audit-heading">Audit Trail</h2>

      <div className="audit-filters" role="group" aria-label="Audit trail filters">
        <label htmlFor="audit-module-filter">
          Module:{' '}
          <select
            id="audit-module-filter"
            value={filter.module ?? ''}
            onChange={(e) => handleModuleFilter(e.target.value)}
          >
            <option value="">All</option>
            <option value="earthworm">Earthworm</option>
            <option value="tlapix">Tlapix</option>
            <option value="ebeecontrol">eBeeControl</option>
            <option value="quack">Quack</option>
            <option value="ollinai">OllinAI</option>
          </select>
        </label>

        <label htmlFor="audit-action-filter">
          Action Type:{' '}
          <select
            id="audit-action-filter"
            value={filter.action_type ?? ''}
            onChange={(e) => handleActionTypeFilter(e.target.value)}
          >
            <option value="">All</option>
            <option value="pod_restart">Pod Restart</option>
            <option value="node_cordon">Node Cordon</option>
            <option value="workload_reschedule">Workload Reschedule</option>
            <option value="isolate_pod">Isolate Pod</option>
            <option value="alert_operator">Alert Operator</option>
            <option value="forensic_report">Forensic Report</option>
          </select>
        </label>
      </div>

      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}
      {loading && <p>Loading...</p>}

      <div className="audit-table-container" role="region" aria-label="Audit trail entries" tabIndex={0}>
        <table className="audit-table">
          <thead>
            <tr>
              <th scope="col">Timestamp</th>
              <th scope="col">Module</th>
              <th scope="col">Action</th>
              <th scope="col">Trigger</th>
              <th scope="col">Confidence</th>
              <th scope="col">Reasoning</th>
              <th scope="col">Outcome</th>
              <th scope="col">Operator</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry, i) => (
              <tr key={`${entry.timestamp}-${i}`}>
                <td>
                  <time dateTime={entry.timestamp}>
                    {new Date(entry.timestamp).toLocaleString()}
                  </time>
                </td>
                <td>{entry.module}</td>
                <td>{entry.action_type}</td>
                <td className="audit-trigger">{entry.trigger_event}</td>
                <td>{(entry.confidence * 100).toFixed(1)}%</td>
                <td className="audit-reasoning">{entry.reasoning}</td>
                <td>{entry.outcome}</td>
                <td>{entry.operator_id || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <nav className="audit-pagination" aria-label="Audit trail pagination">
        <button type="button" onClick={handlePrevPage} disabled={(filter.page ?? 1) <= 1}>
          ← Previous
        </button>
        <span>Page {filter.page ?? 1}</span>
        <button
          type="button"
          onClick={handleNextPage}
          disabled={entries.length < PAGE_SIZE}
        >
          Next →
        </button>
      </nav>
    </section>
  );
}
