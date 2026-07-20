# =============================================================================
# Outputs - TitanOps EKS Infrastructure
# =============================================================================

output "cluster_name" {
  description = "EKS cluster name"
  value       = aws_eks_cluster.main.name
}

output "cluster_endpoint" {
  description = "EKS API server endpoint URL"
  value       = aws_eks_cluster.main.endpoint
}

output "cluster_ca_data" {
  description = "Base64-encoded cluster CA certificate"
  value       = aws_eks_cluster.main.certificate_authority[0].data
}

output "vpc_id" {
  description = "VPC identifier"
  value       = aws_vpc.main.id
}

output "public_subnet_ids" {
  description = "List of public subnet IDs"
  value       = [aws_subnet.public_a.id, aws_subnet.public_b.id]
}

output "private_subnet_ids" {
  description = "List of private subnet IDs"
  value       = [aws_subnet.private_a.id, aws_subnet.private_b.id]
}

output "node_group_role_arn" {
  description = "IAM role ARN for the node group"
  value       = aws_iam_role.node_group.arn
}

output "cluster_security_group_id" {
  description = "Control plane security group ID"
  value       = aws_security_group.eks_control_plane.id
}

output "node_security_group_id" {
  description = "Worker node security group ID"
  value       = aws_security_group.eks_worker_nodes.id
}

output "kubeconfig_command" {
  description = "Ready-to-use aws eks update-kubeconfig command"
  value       = "aws eks update-kubeconfig --region ${var.aws_region} --name ${aws_eks_cluster.main.name} --profile ${var.aws_profile}"
}

# --- Module IRSA Role ARNs ---

output "tlapix_sa_role_arn" {
  description = "IAM role ARN for Tlapix service account (IRSA)"
  value       = var.enable_tlapix ? aws_iam_role.tlapix_sa[0].arn : ""
}

output "earthworm_sa_role_arn" {
  description = "IAM role ARN for Earthworm service account (IRSA)"
  value       = var.enable_earthworm ? aws_iam_role.earthworm_sa[0].arn : ""
}

output "ebeecontrol_sa_role_arn" {
  description = "IAM role ARN for eBeeControl service account (IRSA)"
  value       = var.enable_ebeecontrol ? aws_iam_role.ebeecontrol_sa[0].arn : ""
}

output "quack_sa_role_arn" {
  description = "IAM role ARN for Quack service account (IRSA)"
  value       = var.enable_quack ? aws_iam_role.quack_sa[0].arn : ""
}

output "correlation_sa_role_arn" {
  description = "IAM role ARN for Correlation Engine service account (IRSA)"
  value       = var.enable_correlation ? aws_iam_role.correlation_sa[0].arn : ""
}

# --- Layer 2: Intelligence Layer Outputs ---

output "brainops_sa_role_arn" {
  description = "IAM role ARN for BrainOps service account (IRSA)"
  value       = var.enable_brainops ? aws_iam_role.brainops_sa[0].arn : ""
}

output "cockroachdb_cluster_id" {
  description = "CockroachDB cluster ID"
  value       = var.enable_cockroachdb ? cockroach_cluster.brainops[0].id : ""
}

output "cockroachdb_sql_dns" {
  description = "CockroachDB SQL endpoint (host:port for psql/pg connections)"
  value       = var.enable_cockroachdb ? cockroach_cluster.brainops[0].regions[0].sql_dns : ""
}

output "cockroachdb_ui_url" {
  description = "CockroachDB Cloud Console URL"
  value       = var.enable_cockroachdb ? cockroach_cluster.brainops[0].regions[0].ui_dns : ""
}

output "cockroachdb_secret_arn" {
  description = "ARN of the Secrets Manager secret containing the CockroachDB connection string"
  value       = var.enable_cockroachdb ? aws_secretsmanager_secret.cockroachdb_uri[0].arn : ""
}

output "nats_service_url" {
  description = "In-cluster NATS URL for module connections"
  value       = var.enable_nats ? "nats://nats.titanops.svc.cluster.local:4222" : ""
}
