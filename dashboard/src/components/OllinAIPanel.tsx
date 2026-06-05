import { useEffect, useState } from 'react';
import { fetchOllinAI } from '../api/client';
import type { DeploymentRiskEntry, DORAMetrics } from '../types';

export function OllinAIPanel() {
  const [deployments, setDeployments] = useState<DeploymentRiskEntry[]>([]);
  const [dora, setDora] = useState<DORAMetrics | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    async function poll() {
      try {
        const data = await fetchOllinAI();
        if (active) {
          setDeployments(data.recent_deployments.slice(0, 10));
          setDora(data.dora_metrics);
          setError(null);
        }
      } catch {
        if (active) {
          setError('OllinAI data unavailable');
        }
      }
    }

    poll();
    const interval = setInterval(poll, 30000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, []);

  return (
    <section aria-labelledby="ollinai-panel-heading">
      <h2 id="ollinai-panel-heading">OllinAI — Change Intelligence</h2>

      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}

      <h3>Deployment Risk Scores</h3>
      <ul className="ollinai-risk-list" role="list">
        {deployments.map((d, i) => (
          <li
            key={`${d.commit_sha}-${i}`}
            className={`ollinai-risk-item${d.risk_score >= 80 ? ' ollinai-risk-critical' : ''}`}
          >
            <span className="ollinai-risk-score" aria-label={`Risk score ${d.risk_score}`}>
              {d.risk_score}
            </span>
            <span className="ollinai-risk-service">{d.service}</span>
            <span className="ollinai-risk-deployer">{d.deployer}</span>
            <span className="ollinai-risk-commit" title={d.commit_sha}>
              {d.commit_sha.slice(0, 7)}
            </span>
            <time className="ollinai-risk-time" dateTime={d.timestamp}>
              {new Date(d.timestamp).toLocaleString()}
            </time>
          </li>
        ))}
        {deployments.length === 0 && !error && (
          <li className="ollinai-risk-item">No recent deployments</li>
        )}
      </ul>

      <h3>DORA Metrics</h3>
      {dora ? (
        <dl className="ollinai-dora-grid">
          <div className="ollinai-dora-item">
            <dt>Deployment Frequency</dt>
            <dd>{dora.deployment_frequency.toFixed(1)} / day</dd>
          </div>
          <div className="ollinai-dora-item">
            <dt>Lead Time for Changes</dt>
            <dd>{dora.lead_time_for_changes.toFixed(1)} hrs</dd>
          </div>
          <div className="ollinai-dora-item">
            <dt>Change Failure Rate</dt>
            <dd>{(dora.change_failure_rate * 100).toFixed(1)}%</dd>
          </div>
          <div className="ollinai-dora-item">
            <dt>Mean Time to Recovery</dt>
            <dd>{dora.mean_time_to_recovery.toFixed(1)} hrs</dd>
          </div>
        </dl>
      ) : (
        !error && <p className="ollinai-dora-empty">No DORA metrics available</p>
      )}
    </section>
  );
}
