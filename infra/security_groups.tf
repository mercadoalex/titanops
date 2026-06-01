# =============================================================================
# Security Groups - EKS Control Plane and Worker Nodes
# =============================================================================

# -----------------------------------------------------------------------------
# EKS Control Plane Security Group
# -----------------------------------------------------------------------------

resource "aws_security_group" "eks_control_plane" {
  name        = "${var.cluster_name}-control-plane-sg"
  description = "Security group for EKS control plane"
  vpc_id      = aws_vpc.main.id

  tags = {
    Name      = "${var.cluster_name}-control-plane-sg"
    Component = "security"
  }
}

# Inbound: Allow HTTPS (443) from worker nodes to control plane
resource "aws_security_group_rule" "control_plane_inbound_https" {
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_control_plane.id
  source_security_group_id = aws_security_group.eks_worker_nodes.id
  description              = "Allow HTTPS from worker nodes"
}

# Outbound: Control plane to worker nodes on ports 1025-65535
resource "aws_security_group_rule" "control_plane_outbound_to_workers" {
  type                     = "egress"
  from_port                = 1025
  to_port                  = 65535
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_control_plane.id
  source_security_group_id = aws_security_group.eks_worker_nodes.id
  description              = "Allow outbound to worker nodes for kubelet and kube-proxy"
}

# -----------------------------------------------------------------------------
# Worker Node Security Group
# -----------------------------------------------------------------------------

resource "aws_security_group" "eks_worker_nodes" {
  name        = "${var.cluster_name}-worker-nodes-sg"
  description = "Security group for EKS worker nodes"
  vpc_id      = aws_vpc.main.id

  tags = {
    Name      = "${var.cluster_name}-worker-nodes-sg"
    Component = "security"
  }
}

# Inbound: Allow traffic from control plane on ports 1025-65535
resource "aws_security_group_rule" "workers_inbound_from_control_plane" {
  type                     = "ingress"
  from_port                = 1025
  to_port                  = 65535
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_worker_nodes.id
  source_security_group_id = aws_security_group.eks_control_plane.id
  description              = "Allow inbound from control plane for kubelet and kube-proxy"
}

# Inbound: Self-referencing rule for inter-node communication (all ports, all protocols)
resource "aws_security_group_rule" "workers_inbound_self" {
  type              = "ingress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.eks_worker_nodes.id
  self              = true
  description       = "Allow all inter-node communication (required for module-to-module traffic)"
}

# Inbound: NodePort range for external access (30000-32767)
resource "aws_security_group_rule" "workers_inbound_nodeport" {
  type              = "ingress"
  from_port         = 30000
  to_port           = 32767
  protocol          = "tcp"
  security_group_id = aws_security_group.eks_worker_nodes.id
  cidr_blocks       = [var.nodeport_cidr]
  description       = "Allow NodePort access for external traffic"
}

# Outbound: Allow all traffic from worker nodes
resource "aws_security_group_rule" "workers_outbound_all" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  security_group_id = aws_security_group.eks_worker_nodes.id
  cidr_blocks       = ["0.0.0.0/0"]
  description       = "Allow all outbound traffic"
}
