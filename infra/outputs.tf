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
