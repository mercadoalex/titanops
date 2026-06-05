import { useEffect, useState } from 'react';
import { fetchCorrelations } from '../api/client';
import type { CorrelatedIncident } from '../types';
import { OverrideControls } from './OverrideControls';

export function CorrelationTimeline() {
  const [incidents, setIncidents] = useState<CorrelatedIncident[]>([]);
  const [windowMinutes, setWindowMinutes] = useState(60);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      try {
        const data = await fetchCorrelations(windowMinutes);
        if (active) {
          setIncidents(data);
          setError(null);
        }
      } catch (err) {
        if (active)
          setError(err instanceof Error ? err.message : 'Failed to fetch correlations');
      }
    }

    load();
    const interval = setInterval(load, 15000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, [windowMinutes]);

  return (
    <section aria-labelledby="correlation-heading">
      <h2 id="correlation-heading">Correlation Timeline</h2>

      <label htmlFor="time-window-select">
        Time Window:{' '}
        <select
          id="time-window-select"
          value={windowMinutes}
          onChange={(e) => setWindowMinutes(Number(e.target.value))}
        >
          <option value={15}>15 min</option>
          <option value={30}>30 min</option>
          <option value={60}>60 min</option>
          <option value={120}>2 hours</option>
          <option value={360}>6 hours</option>
        </select>
      </label>

      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}

      <ol className="correlation-list" role="list">
        {incidents.map((incident) => (
          <li key={incident.incident_id} className="correlation-item">
            <header className="correlation-header">
              <span className="correlation-id">{incident.incident_id.slice(0, 8)}</span>
              <span className="correlation-confidence">
                Confidence: {incident.confidence}%
              </span>
              <time dateTime={incident.detected_at}>
                {new Date(incident.detected_at).toLocaleString()}
              </time>
            </header>
            <p className="correlation-narrative">{incident.narrative}</p>
            {incident.modules.includes('ollinai') && incident.deployment_metadata && (
              <dl className="deployment-metadata" aria-label="Deployment metadata">
                {incident.deployment_metadata.service && (
                  <>
                    <dt>Service</dt>
                    <dd>{incident.deployment_metadata.service}</dd>
                  </>
                )}
                {incident.deployment_metadata.commit_sha && (
                  <>
                    <dt>Commit</dt>
                    <dd>
                      <code>{incident.deployment_metadata.commit_sha.slice(0, 8)}</code>
                    </dd>
                  </>
                )}
                {incident.deployment_metadata.deployer && (
                  <>
                    <dt>Deployer</dt>
                    <dd>{incident.deployment_metadata.deployer}</dd>
                  </>
                )}
              </dl>
            )}
            <ul className="correlation-modules" aria-label="Contributing modules">
              {incident.modules.map((mod) => (
                <li key={mod} className="module-tag">
                  {mod}
                </li>
              ))}
            </ul>
            <p className="matched-attrs">
              Matched: {incident.matched_attributes.join(', ')}
            </p>
            {incident.action_taken && (
              <p className="correlation-action">
                Action taken — outcome: {incident.action_outcome ?? 'unknown'}
              </p>
            )}
            {!incident.action_taken && (
              <OverrideControls
                actionId={incident.incident_id}
                moduleId={incident.modules[0] ?? ''}
              />
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
