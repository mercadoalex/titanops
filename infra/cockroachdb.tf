# =============================================================================
# CockroachDB Serverless - BrainOps Agent Memory Layer
# =============================================================================
# Only provisioned when enable_cockroachdb = true.
# Provides: persistent agent memory, vector search, checkpoint storage,
# incident history, resolution playbooks, audit trail.
# =============================================================================

# -----------------------------------------------------------------------------
# CockroachDB Serverless Cluster
# -----------------------------------------------------------------------------

resource "cockroach_cluster" "brainops" {
  count = var.enable_cockroachdb ? 1 : 0

  name       = "${var.project_name}-${var.environment}"
  cloud      = "AWS"
  plan       = var.cockroachdb_plan

  serverless = {
    spend_limit = var.cockroachdb_spend_limit
  }

  regions = [{
    name = var.aws_region
  }]
}

# -----------------------------------------------------------------------------
# Database for BrainOps agent memory
# -----------------------------------------------------------------------------

resource "cockroach_database" "ollinai" {
  count = var.enable_cockroachdb ? 1 : 0

  name       = "ollinai"
  cluster_id = cockroach_cluster.brainops[0].id
}

# -----------------------------------------------------------------------------
# SQL User for BrainOps agent
# -----------------------------------------------------------------------------

resource "cockroach_sql_user" "brainops" {
  count = var.enable_cockroachdb ? 1 : 0

  name       = var.cockroachdb_sql_user
  password   = var.cockroachdb_sql_password
  cluster_id = cockroach_cluster.brainops[0].id
}

# -----------------------------------------------------------------------------
# Service Account for ccloud CLI access (BrainOps self-healing)
# -----------------------------------------------------------------------------

resource "cockroach_service_account" "brainops_agent" {
  count = var.enable_cockroachdb ? 1 : 0

  name        = "${var.project_name}-brainops-agent"
  description = "Service account for BrainOps agent ccloud CLI access"
}

# -----------------------------------------------------------------------------
# Store connection string in AWS Secrets Manager
# (BrainOps pods retrieve this via IRSA + Secrets Store CSI Driver)
# -----------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "cockroachdb_uri" {
  count = var.enable_cockroachdb ? 1 : 0

  name        = "${var.project_name}/${var.environment}/cockroachdb-uri"
  description = "CockroachDB connection string for BrainOps agent"

  tags = {
    Component = "brainops"
  }
}

resource "aws_secretsmanager_secret_version" "cockroachdb_uri" {
  count = var.enable_cockroachdb ? 1 : 0

  secret_id = aws_secretsmanager_secret.cockroachdb_uri[0].id
  secret_string = jsonencode({
    uri          = "postgresql://${var.cockroachdb_sql_user}:${var.cockroachdb_sql_password}@${cockroach_cluster.brainops[0].regions[0].sql_dns}:26257/ollinai?sslmode=verify-full"
    cluster_id   = cockroach_cluster.brainops[0].id
    sql_dns      = cockroach_cluster.brainops[0].regions[0].sql_dns
    http_dns     = cockroach_cluster.brainops[0].regions[0].ui_dns
    database     = "ollinai"
    user         = var.cockroachdb_sql_user
  })
}

# -----------------------------------------------------------------------------
# IRSA Role for BrainOps (access to Secrets Manager + Bedrock)
# -----------------------------------------------------------------------------

resource "aws_iam_role" "brainops_sa" {
  count = var.enable_brainops ? 1 : 0

  name = "${var.cluster_name}-brainops-sa"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.eks.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:sub" = "system:serviceaccount:titanops:brainops"
          "${replace(aws_eks_cluster.main.identity[0].oidc[0].issuer, "https://", "")}:aud" = "sts.amazonaws.com"
        }
      }
    }]
  })

  tags = {
    Component = "brainops"
  }
}

resource "aws_iam_role_policy" "brainops_secrets" {
  count = var.enable_brainops ? 1 : 0

  name = "brainops-secrets-access"
  role = aws_iam_role.brainops_sa[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = var.enable_cockroachdb ? [aws_secretsmanager_secret.cockroachdb_uri[0].arn] : []
      },
      {
        Effect = "Allow"
        Action = [
          "bedrock:InvokeModel",
          "bedrock:InvokeModelWithResponseStream"
        ]
        Resource = "arn:aws:bedrock:${var.aws_region}::foundation-model/*"
      }
    ]
  })
}
