-- BrainOps CockroachDB Schema Migration
-- Creates the ollinai schema with all tables for agent persistent memory.
-- Idempotent: safe to run multiple times.

-- =============================================================================
-- Schema
-- =============================================================================

CREATE SCHEMA IF NOT EXISTS ollinai;

-- =============================================================================
-- Table 1: checkpoints — LangGraph state persistence
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.checkpoints (
    thread_id TEXT NOT NULL,
    checkpoint_ns TEXT NOT NULL DEFAULT '',
    tenant_id UUID NOT NULL,
    parent_checkpoint_id TEXT,
    checkpoint JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (tenant_id, thread_id, checkpoint_ns)
);

-- =============================================================================
-- Table 2: incidents — Historical incidents from correlation engine
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    correlation_id UUID,
    modules TEXT[] NOT NULL,
    severity INT NOT NULL CHECK (severity BETWEEN 1 AND 5),
    narrative TEXT NOT NULL,
    contributing_events JSONB NOT NULL,
    confidence_score INT NOT NULL CHECK (confidence_score BETWEEN 0 AND 100),
    resolution_action TEXT,
    resolution_time_ms INT,
    node_id TEXT,
    namespace TEXT,
    pod_name TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    resolved_at TIMESTAMPTZ
) WITH (ttl_expiration_expression = '((created_at) + ''90 days'':::INTERVAL)', ttl_job_cron = '@daily');

CREATE INDEX IF NOT EXISTS idx_incidents_tenant_created
    ON ollinai.incidents (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_incidents_tenant_node_created
    ON ollinai.incidents (tenant_id, node_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_incidents_tenant_namespace_created
    ON ollinai.incidents (tenant_id, namespace, created_at DESC);

-- =============================================================================
-- Table 3: incident_embeddings — Vector similarity search
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.incident_embeddings (
    incident_id UUID NOT NULL REFERENCES ollinai.incidents(id),
    tenant_id UUID NOT NULL,
    embedding VECTOR(1536) NOT NULL,
    model_version TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (tenant_id, incident_id)
);

CREATE INDEX IF NOT EXISTS idx_incident_embeddings_vector
    ON ollinai.incident_embeddings
    USING vectorsearch (embedding vector_cosine_ops)
    WITH (lists = 100);

-- =============================================================================
-- Table 4: resolutions — Resolution playbooks
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.resolutions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    incident_id UUID REFERENCES ollinai.incidents(id),
    action_sequence JSONB NOT NULL,
    success BOOLEAN NOT NULL,
    duration_ms INT NOT NULL,
    context JSONB,
    reuse_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_resolutions_tenant_success_created
    ON ollinai.resolutions (tenant_id, success, created_at DESC);

-- =============================================================================
-- Table 5: deployments — Deployment risk history
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    service_name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    deployer TEXT NOT NULL,
    risk_score INT NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
    risk_factors JSONB NOT NULL,
    caused_incident BOOLEAN DEFAULT false,
    incident_id UUID REFERENCES ollinai.incidents(id),
    deployed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_deployments_tenant_service_deployed
    ON ollinai.deployments (tenant_id, service_name, deployed_at DESC);

CREATE INDEX IF NOT EXISTS idx_deployments_tenant_risk
    ON ollinai.deployments (tenant_id, risk_score DESC);

-- =============================================================================
-- Table 6: audit_log — Every autonomous action with reasoning
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    module TEXT NOT NULL,
    action_type TEXT NOT NULL,
    target TEXT NOT NULL,
    trigger_event_id TEXT,
    confidence FLOAT NOT NULL,
    reasoning JSONB NOT NULL,
    outcome TEXT NOT NULL,
    operator_id TEXT,
    duration_ms INT,
    created_at TIMESTAMPTZ DEFAULT now()
) WITH (ttl_expiration_expression = '((created_at) + ''365 days'':::INTERVAL)', ttl_job_cron = '@daily');

CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_created
    ON ollinai.audit_log (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_module_created
    ON ollinai.audit_log (tenant_id, module, created_at DESC);

-- =============================================================================
-- Table 7: cert_ledger — Tlapix certificate tracking
-- =============================================================================

CREATE TABLE IF NOT EXISTS ollinai.cert_ledger (
    fingerprint TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    service_name TEXT NOT NULL,
    namespace TEXT NOT NULL,
    expiry_date TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    kms_key_id TEXT,
    renewal_count INT DEFAULT 0,
    last_renewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (tenant_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_cert_ledger_tenant_expiry
    ON ollinai.cert_ledger (tenant_id, expiry_date ASC);

-- =============================================================================
-- Row-Level Security (RLS) — Tenant isolation
-- =============================================================================

ALTER TABLE ollinai.checkpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.checkpoints FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.checkpoints
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.incidents ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.incidents FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.incidents
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.incident_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.incident_embeddings FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.incident_embeddings
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.resolutions ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.resolutions FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.resolutions
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.deployments ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.deployments FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.deployments
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.audit_log
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);

ALTER TABLE ollinai.cert_ledger ENABLE ROW LEVEL SECURITY;
ALTER TABLE ollinai.cert_ledger FORCE ROW LEVEL SECURITY;
CREATE POLICY IF NOT EXISTS tenant_iso ON ollinai.cert_ledger
    FOR ALL USING (tenant_id = current_setting('app.tenant_id')::UUID);
