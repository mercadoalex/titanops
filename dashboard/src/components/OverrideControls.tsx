import { useState } from 'react';
import { postOverride } from '../api/client';
import type { OverrideRequest } from '../types';

interface OverrideControlsProps {
  actionId: string;
  moduleId: string;
}

type Operation = OverrideRequest['operation'];

export function OverrideControls({ actionId, moduleId }: OverrideControlsProps) {
  const [pending, setPending] = useState<Operation | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  async function handleConfirm() {
    if (!pending) return;
    setSubmitting(true);
    setError(null);
    setSuccess(null);

    try {
      await postOverride({
        action_id: actionId,
        module_id: moduleId,
        operation: pending,
        operator_id: 'operator', // would come from auth in production
      });
      setSuccess(`Action ${pending}d successfully`);
      setPending(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Override failed');
    } finally {
      setSubmitting(false);
    }
  }

  function handleCancel() {
    setPending(null);
    setError(null);
  }

  return (
    <div className="override-controls" role="group" aria-label="Override controls">
      {error && (
        <p role="alert" className="error-message">
          {error}
        </p>
      )}
      {success && (
        <p role="status" className="success-message">
          {success}
        </p>
      )}

      {!pending && (
        <div className="override-buttons">
          <button type="button" onClick={() => setPending('approve')} className="btn-approve">
            Approve
          </button>
          <button type="button" onClick={() => setPending('reject')} className="btn-reject">
            Reject
          </button>
          <button type="button" onClick={() => setPending('pause')} className="btn-pause">
            Pause
          </button>
        </div>
      )}

      {pending && (
        <div className="override-confirm" role="alertdialog" aria-describedby="confirm-message">
          <p id="confirm-message">
            Confirm <strong>{pending}</strong> for action {actionId.slice(0, 8)}?
          </p>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={submitting}
            className="btn-confirm"
          >
            {submitting ? 'Submitting...' : 'Confirm'}
          </button>
          <button
            type="button"
            onClick={handleCancel}
            disabled={submitting}
            className="btn-cancel"
          >
            Cancel
          </button>
        </div>
      )}
    </div>
  );
}
