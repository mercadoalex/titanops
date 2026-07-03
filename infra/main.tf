terraform {
  required_version = ">= 1.7.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.50"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    cockroach = {
      source  = "cockroachdb/cockroach"
      version = "~> 1.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.14"
    }
  }

  backend "s3" {
    bucket         = "titanops-terraform-state-340341089363"
    key            = "eks/terraform.tfstate"
    region         = "us-east-2"
    dynamodb_table = "titanops-terraform-locks"
    encrypt        = true
    profile        = "experiment"
  }
}

provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile

  default_tags {
    tags = {
      Project     = var.project_name
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

provider "helm" {
  kubernetes {
    host                   = aws_eks_cluster.main.endpoint
    cluster_ca_certificate = base64decode(aws_eks_cluster.main.certificate_authority[0].data)
    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", aws_eks_cluster.main.name, "--region", var.aws_region, "--profile", var.aws_profile]
    }
  }
}

provider "cockroach" {
  # API key from CockroachDB Cloud Console
  # Set via: export COCKROACH_API_KEY=... or var.cockroachdb_api_key
  apikey = var.cockroachdb_api_key
}
