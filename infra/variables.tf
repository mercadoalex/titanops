variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-east-2"
}

variable "aws_profile" {
  description = "AWS CLI profile to use for authentication"
  type        = string
  default     = "experiment"
}

variable "project_name" {
  description = "Project name for tagging"
  type        = string
  default     = "titanops"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
  default     = "titanops-eks"
}

variable "cluster_version" {
  description = "Kubernetes version"
  type        = string
  default     = "1.32"

  validation {
    condition     = can(regex("^[0-9]+\\.[0-9]+$", var.cluster_version))
    error_message = "cluster_version must be in the format 'MAJOR.MINOR' (e.g., 1.32)."
  }
}

variable "vpc_cidr" {
  description = "VPC CIDR block"
  type        = string
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrhost(var.vpc_cidr, 0))
    error_message = "vpc_cidr must be a valid CIDR block (e.g., 10.0.0.0/16)."
  }
}

variable "node_instance_types" {
  description = "EC2 instance types for nodes"
  type        = list(string)
  default     = ["m5.xlarge"]
}

variable "node_min_size" {
  description = "Minimum node count"
  type        = number
  default     = 2

  validation {
    condition     = var.node_min_size >= 1
    error_message = "node_min_size must be at least 1."
  }
}

variable "node_desired_size" {
  description = "Desired node count"
  type        = number
  default     = 3

  validation {
    condition     = var.node_desired_size >= 1
    error_message = "node_desired_size must be at least 1."
  }
}

variable "node_max_size" {
  description = "Maximum node count"
  type        = number
  default     = 5

  validation {
    condition     = var.node_max_size >= 1
    error_message = "node_max_size must be at least 1."
  }
}

variable "node_disk_size" {
  description = "Root volume size in GiB"
  type        = number
  default     = 50

  validation {
    condition     = var.node_disk_size >= 20
    error_message = "node_disk_size must be at least 20 GiB."
  }
}

variable "nodeport_cidr" {
  description = "CIDR allowed to access NodePort range for external traffic (benchmarks, testing)"
  type        = string
  default     = "0.0.0.0/0"

  validation {
    condition     = can(cidrhost(var.nodeport_cidr, 0))
    error_message = "nodeport_cidr must be a valid CIDR block (e.g., 10.0.0.0/8 or 0.0.0.0/0)."
  }
}

variable "custom_ami_id" {
  description = "Custom AMI ID with kernel 6.12+ for sched_ext support (required by Quack). If empty, uses AL2023 standard."
  type        = string
  default     = ""
}

# -----------------------------------------------------------------------------
# Module enablement flags (controls IRSA role creation)
# -----------------------------------------------------------------------------

variable "enable_tlapix" {
  description = "Create IRSA role for Tlapix module"
  type        = bool
  default     = true
}

variable "enable_earthworm" {
  description = "Create IRSA role for Earthworm module"
  type        = bool
  default     = true
}

variable "enable_ebeecontrol" {
  description = "Create IRSA role for eBeeControl module"
  type        = bool
  default     = true
}

variable "enable_quack" {
  description = "Create IRSA role for Quack module"
  type        = bool
  default     = true
}

variable "enable_correlation" {
  description = "Create IRSA role for Correlation Engine"
  type        = bool
  default     = true
}
