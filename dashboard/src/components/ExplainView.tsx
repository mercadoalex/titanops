import { useEffect, useState } from 'react';
import { fetchExplain } from '../api/client';
import type { ExplainDetail } from '../types';

interface ExplainViewProps {
  actionId: string | null;
  onClose: () => void;
}

export function ExplainView({ actionId, onClose }: ExplainViewProps) {
  const [detail, setDetail] = useState<ExplainDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!actionId) {
      setDetail(null);
      return;
    }

    let active = true;
    setLoading(true);

    fetchExplain(actionId)
      .then((data) => {
        if (active) {
          setDetail(data);
          setError(null);
        }
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : 'Failed to fetch explanation');
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [actionId]);

  if (!actionId) return null;

  return (
    <aside className="explain-view" aria-labelledby="explain-heading" role="complementary">
      <header className="explain-header">
        <h3 id="explain-heading">Action Explanation</h3>
        <button type="button" onClick={onClose} aria-label="Close explanation">
          ✕
        </button>
      </header>

      {loading && <p>Loading...</p>}
      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}

      {detail && (
        <div className="explain-content">
          <dl>
            <dt>Module</dt>
            <dd>{detail.module}</dd>
            <dt>Confidence</dt>
            <dd>{(detail.confidence * 100).toFixed(1)}%</dd>
            <dt>Trigger Event</dt>
            <dd>{detail.trigger_event}</dd>
            <dt>Timestamp</dt>
            <dd>
              <time dateTime={detail.timestamp}>
                {new Date(detail.timestamp).toLocaleString()}
              </time>
            </dd>
          </dl>

          {detail.module === 'ollinai' && detail.deployment_metadata && (
            <div className="deployment-details">
              <h4>Deployment Details</h4>
              <dl>
                {detail.deployment_metadata.service && (
                  <>
                    <dt>Service</dt>
                    <dd>{detail.deployment_metadata.service}</dd>
                  </>
                )}
                {detail.deployment_metadata.commit_sha && (
                  <>
                    <dt>Commit SHA</dt>
                    <dd>
                      <code>{detail.deployment_metadata.commit_sha}</code>
                    </dd>
                  </>
                )}
                {detail.deployment_metadata.deployer && (
                  <>
                    <dt>Deployer</dt>
                    <dd>{detail.deployment_metadata.deployer}</dd>
                  </>
                )}
                {detail.deployment_metadata.risk_score !== undefined && (
                  <>
                    <dt>Risk Score</dt>
                    <dd
                      className={
                        detail.deployment_metadata.risk_score >= 80
                          ? 'risk-critical'
                          : detail.deployment_metadata.risk_score >= 60
                            ? 'risk-high'
                            : ''
                      }
                    >
                      {detail.deployment_metadata.risk_score}
                    </dd>
                  </>
                )}
                {detail.deployment_metadata.risk_factors &&
                  detail.deployment_metadata.risk_factors.length > 0 && (
                    <>
                      <dt>Risk Factors</dt>
                      <dd>
                        <ul>
                          {detail.deployment_metadata.risk_factors.map((factor, i) => (
                            <li key={i}>{factor}</li>
                          ))}
                        </ul>
                      </dd>
                    </>
                  )}
              </dl>
            </div>
          )}

          <h4>Reasoning Chain</h4>
          <dl className="reasoning-chain">
            <dt>Observation</dt>
            <dd>{detail.reasoning.observation}</dd>
            <dt>Analysis</dt>
            <dd>{detail.reasoning.analysis}</dd>
            <dt>Action</dt>
            <dd>{detail.reasoning.action}</dd>
          </dl>

          {detail.reasoning.alternatives && detail.reasoning.alternatives.length > 0 && (
            <>
              <h4>Alternatives Considered</h4>
              <ul>
                {detail.reasoning.alternatives.map((alt, i) => (
                  <li key={i}>{alt}</li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
    </aside>
  );
}
