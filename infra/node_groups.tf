# =============================================================================
# EKS Worker Nodes
#
# When custom_ami_id is set: uses a self-managed ASG with kernel 6.12 + sched_ext.
# When custom_ami_id is empty: uses a managed node group with stock AL2023.
#
# The custom AMI path is required by the Quack module (sched_ext scheduler).
# All other modules run fine on standard AL2023 nodes.
# =============================================================================

locals {
  use_custom_ami = var.custom_ami_id != ""
}

# =============================================================================
# Instance Profile (needed for self-managed nodes)
# =============================================================================

resource "aws_iam_instance_profile" "node" {
  name = "${var.cluster_name}-node-instance-profile"
  role = aws_iam_role.node_group.name

  tags = {
    Component = "security"
  }
}

# =============================================================================
# Option A: Self-Managed ASG with Custom AMI (kernel 6.12 + sched_ext)
# Required by Quack module for sched_ext support
# =============================================================================

resource "aws_launch_template" "scx_nodes" {
  count = local.use_custom_ami ? 1 : 0

  name_prefix   = "${var.cluster_name}-scx-nodes-"
  image_id      = var.custom_ami_id
  instance_type = var.node_instance_types[0]

  iam_instance_profile {
    arn = aws_iam_instance_profile.node.arn
  }

  vpc_security_group_ids = [
    aws_security_group.eks_worker_nodes.id,
  ]

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size = 100
      volume_type = "gp3"
    }
  }

  # nodeadm user data for AL2023 EKS AMI (joins the cluster automatically)
  user_data = base64encode(<<-EOF
    ---
    apiVersion: node.eks.aws/v1alpha1
    kind: NodeConfig
    spec:
      cluster:
        name: ${aws_eks_cluster.main.name}
        apiServerEndpoint: ${aws_eks_cluster.main.endpoint}
        certificateAuthority: ${aws_eks_cluster.main.certificate_authority[0].data}
        cidr: ${aws_eks_cluster.main.kubernetes_network_config[0].service_ipv4_cidr}
      kubelet:
        config:
          clusterDNS:
            - 172.20.0.10
        flags:
          - --node-labels=scheduler-capable=true,kernel=6.12-scx,titanops.io/module=quack
  EOF
  )

  tag_specifications {
    resource_type = "instance"

    tags = {
      Name      = "${var.cluster_name}-scx-node"
      Component = "compute"
      Kernel    = "6.12-sched_ext"
    }
  }

  tags = {
    Name      = "${var.cluster_name}-scx-launch-template"
    Component = "compute"
  }
}

resource "aws_autoscaling_group" "scx_nodes" {
  count = local.use_custom_ami ? 1 : 0

  name_prefix      = "${var.cluster_name}-scx-nodes-"
  min_size         = var.node_min_size
  desired_capacity = var.node_desired_size
  max_size         = var.node_max_size

  vpc_zone_identifier = [
    aws_subnet.private_a.id,
    aws_subnet.private_b.id,
  ]

  launch_template {
    id      = aws_launch_template.scx_nodes[0].id
    version = aws_launch_template.scx_nodes[0].latest_version
  }

  tag {
    key                 = "Name"
    value               = "${var.cluster_name}-scx-node"
    propagate_at_launch = true
  }

  tag {
    key                 = "kubernetes.io/cluster/${var.cluster_name}"
    value               = "owned"
    propagate_at_launch = true
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_worker_policy,
    aws_iam_role_policy_attachment.node_cni_policy,
    aws_iam_role_policy_attachment.node_registry_policy,
  ]
}

# =============================================================================
# Option B: Managed Node Group with Stock AL2023 (default, all modules)
# =============================================================================

resource "aws_launch_template" "nodes" {
  count = local.use_custom_ami ? 0 : 1

  name_prefix = "${var.cluster_name}-nodes-"

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size = var.node_disk_size
      volume_type = "gp3"
    }
  }

  tag_specifications {
    resource_type = "instance"

    tags = {
      Name      = "${var.cluster_name}-node"
      Component = "compute"
    }
  }

  tags = {
    Name      = "${var.cluster_name}-launch-template"
    Component = "compute"
  }
}

resource "aws_eks_node_group" "main" {
  count = local.use_custom_ami ? 0 : 1

  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "${var.cluster_name}-nodes"
  node_role_arn   = aws_iam_role.node_group.arn

  subnet_ids = [
    aws_subnet.private_a.id,
    aws_subnet.private_b.id,
  ]

  ami_type       = "AL2023_x86_64_STANDARD"
  instance_types = var.node_instance_types

  scaling_config {
    min_size     = var.node_min_size
    desired_size = var.node_desired_size
    max_size     = var.node_max_size
  }

  launch_template {
    id      = aws_launch_template.nodes[0].id
    version = aws_launch_template.nodes[0].latest_version
  }

  labels = {
    "titanops.io/platform" = "true"
  }

  tags = {
    Name      = "${var.cluster_name}-node-group"
    Component = "compute"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_worker_policy,
    aws_iam_role_policy_attachment.node_cni_policy,
    aws_iam_role_policy_attachment.node_registry_policy,
  ]
}
