# =============================================================================
# IAM Roles and Policies for EKS Infrastructure
# =============================================================================

# --- EKS Cluster Role ---

data "aws_iam_policy_document" "eks_cluster_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "eks_cluster" {
  name               = "${var.cluster_name}-cluster-role"
  assume_role_policy = data.aws_iam_policy_document.eks_cluster_assume_role.json

  tags = {
    Component = "security"
  }
}

resource "aws_iam_role_policy_attachment" "eks_cluster_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.eks_cluster.name
}

# --- Node Group Role ---

data "aws_iam_policy_document" "node_group_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "node_group" {
  name               = "${var.cluster_name}-node-role"
  assume_role_policy = data.aws_iam_policy_document.node_group_assume_role.json

  tags = {
    Component = "security"
  }
}

resource "aws_iam_role_policy_attachment" "node_worker_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.node_group.name
}

resource "aws_iam_role_policy_attachment" "node_cni_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.node_group.name
}

resource "aws_iam_role_policy_attachment" "node_registry_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.node_group.name
}

resource "aws_iam_role_policy_attachment" "node_ssm_policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
  role       = aws_iam_role.node_group.name
}

# --- OIDC Provider for IRSA ---

data "aws_caller_identity" "current" {}

data "tls_certificate" "eks" {
  url = aws_eks_cluster.main.identity[0].oidc[0].issuer
}

resource "aws_iam_openid_connect_provider" "eks" {
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.eks.certificates[0].sha1_fingerprint]
  url             = aws_eks_cluster.main.identity[0].oidc[0].issuer

  tags = {
    Component = "security"
  }
}

locals {
  oidc_provider_arn = aws_iam_openid_connect_provider.eks.arn
  oidc_provider_url = replace(aws_iam_openid_connect_provider.eks.url, "https://", "")
}

# =============================================================================
# Module IRSA Roles
# Each TitanOps module gets its own service account IAM role
# =============================================================================

# --- Tlapix Service Account ---

data "aws_iam_policy_document" "tlapix_sa_assume_role" {
  count = var.enable_tlapix ? 1 : 0

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:titanops:tlapix-sa"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "tlapix_sa" {
  count = var.enable_tlapix ? 1 : 0

  name               = "${var.cluster_name}-tlapix-sa-role"
  assume_role_policy = data.aws_iam_policy_document.tlapix_sa_assume_role[0].json

  tags = {
    Component = "security"
    Module    = "tlapix"
  }
}

resource "aws_iam_policy" "tlapix_sa" {
  count = var.enable_tlapix ? 1 : 0

  name        = "${var.cluster_name}-tlapix-sa-policy"
  description = "Policy for Tlapix - certificate management and secrets access"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/titanops/tlapix/*"
      },
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:titanops/tlapix/*"
      }
    ]
  })

  tags = {
    Component = "security"
    Module    = "tlapix"
  }
}

resource "aws_iam_role_policy_attachment" "tlapix_sa" {
  count = var.enable_tlapix ? 1 : 0

  policy_arn = aws_iam_policy.tlapix_sa[0].arn
  role       = aws_iam_role.tlapix_sa[0].name
}

# --- Earthworm Service Account ---

data "aws_iam_policy_document" "earthworm_sa_assume_role" {
  count = var.enable_earthworm ? 1 : 0

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:titanops:earthworm-sa"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "earthworm_sa" {
  count = var.enable_earthworm ? 1 : 0

  name               = "${var.cluster_name}-earthworm-sa-role"
  assume_role_policy = data.aws_iam_policy_document.earthworm_sa_assume_role[0].json

  tags = {
    Component = "security"
    Module    = "earthworm"
  }
}

resource "aws_iam_policy" "earthworm_sa" {
  count = var.enable_earthworm ? 1 : 0

  name        = "${var.cluster_name}-earthworm-sa-policy"
  description = "Policy for Earthworm - cluster health monitoring and logging"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/titanops/earthworm/*"
      }
    ]
  })

  tags = {
    Component = "security"
    Module    = "earthworm"
  }
}

resource "aws_iam_role_policy_attachment" "earthworm_sa" {
  count = var.enable_earthworm ? 1 : 0

  policy_arn = aws_iam_policy.earthworm_sa[0].arn
  role       = aws_iam_role.earthworm_sa[0].name
}

# --- eBeeControl Service Account ---

data "aws_iam_policy_document" "ebeecontrol_sa_assume_role" {
  count = var.enable_ebeecontrol ? 1 : 0

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:titanops:ebeecontrol-sa"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ebeecontrol_sa" {
  count = var.enable_ebeecontrol ? 1 : 0

  name               = "${var.cluster_name}-ebeecontrol-sa-role"
  assume_role_policy = data.aws_iam_policy_document.ebeecontrol_sa_assume_role[0].json

  tags = {
    Component = "security"
    Module    = "ebeecontrol"
  }
}

resource "aws_iam_policy" "ebeecontrol_sa" {
  count = var.enable_ebeecontrol ? 1 : 0

  name        = "${var.cluster_name}-ebeecontrol-sa-policy"
  description = "Policy for eBeeControl - threat detection, honeytokens, and logging"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/titanops/ebeecontrol/*"
      },
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:titanops/ebeecontrol/*"
      }
    ]
  })

  tags = {
    Component = "security"
    Module    = "ebeecontrol"
  }
}

resource "aws_iam_role_policy_attachment" "ebeecontrol_sa" {
  count = var.enable_ebeecontrol ? 1 : 0

  policy_arn = aws_iam_policy.ebeecontrol_sa[0].arn
  role       = aws_iam_role.ebeecontrol_sa[0].name
}

# --- Quack Service Account ---

data "aws_iam_policy_document" "quack_sa_assume_role" {
  count = var.enable_quack ? 1 : 0

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:titanops:quack-sa"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "quack_sa" {
  count = var.enable_quack ? 1 : 0

  name               = "${var.cluster_name}-quack-sa-role"
  assume_role_policy = data.aws_iam_policy_document.quack_sa_assume_role[0].json

  tags = {
    Component = "security"
    Module    = "quack"
  }
}

resource "aws_iam_policy" "quack_sa" {
  count = var.enable_quack ? 1 : 0

  name        = "${var.cluster_name}-quack-sa-policy"
  description = "Policy for Quack - CPU scheduling and logging"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/titanops/quack/*"
      }
    ]
  })

  tags = {
    Component = "security"
    Module    = "quack"
  }
}

resource "aws_iam_role_policy_attachment" "quack_sa" {
  count = var.enable_quack ? 1 : 0

  policy_arn = aws_iam_policy.quack_sa[0].arn
  role       = aws_iam_role.quack_sa[0].name
}

# --- Correlation Engine Service Account ---

data "aws_iam_policy_document" "correlation_sa_assume_role" {
  count = var.enable_correlation ? 1 : 0

  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [local.oidc_provider_arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:sub"
      values   = ["system:serviceaccount:titanops:correlation-sa"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "correlation_sa" {
  count = var.enable_correlation ? 1 : 0

  name               = "${var.cluster_name}-correlation-sa-role"
  assume_role_policy = data.aws_iam_policy_document.correlation_sa_assume_role[0].json

  tags = {
    Component = "security"
    Module    = "correlation"
  }
}

resource "aws_iam_policy" "correlation_sa" {
  count = var.enable_correlation ? 1 : 0

  name        = "${var.cluster_name}-correlation-sa-policy"
  description = "Policy for Correlation Engine - cross-module event processing and logging"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:log-group:/titanops/correlation/*"
      }
    ]
  })

  tags = {
    Component = "security"
    Module    = "correlation"
  }
}

resource "aws_iam_role_policy_attachment" "correlation_sa" {
  count = var.enable_correlation ? 1 : 0

  policy_arn = aws_iam_policy.correlation_sa[0].arn
  role       = aws_iam_role.correlation_sa[0].name
}
