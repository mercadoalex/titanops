# =============================================================================
# NATS Event Bus - In-Cluster Pub/Sub for Cross-Module Correlation
# =============================================================================
# Only deployed when enable_nats = true.
# Provides: real-time event routing between modules and the correlation engine.
# Cost: $0 (in-cluster pod, ~15MB RAM)
# =============================================================================

resource "helm_release" "nats" {
  count = var.enable_nats ? 1 : 0

  name       = "nats"
  repository = "https://nats-io.github.io/k8s/helm/charts/"
  chart      = "nats"
  version    = "1.2.6"
  namespace  = "titanops"

  create_namespace = true

  # Minimal configuration for event bus use case
  set {
    name  = "config.cluster.enabled"
    value = "false"  # Single instance for dev; enable for HA
  }

  set {
    name  = "config.jetstream.enabled"
    value = "false"  # Enable if you need event replay/persistence
  }

  set {
    name  = "container.image.tag"
    value = "2.10-alpine"
  }

  set {
    name  = "reloader.enabled"
    value = "true"
  }

  # Resource limits (NATS is very lightweight)
  set {
    name  = "container.resources.requests.cpu"
    value = "50m"
  }

  set {
    name  = "container.resources.requests.memory"
    value = "32Mi"
  }

  set {
    name  = "container.resources.limits.cpu"
    value = "200m"
  }

  set {
    name  = "container.resources.limits.memory"
    value = "64Mi"
  }

  depends_on = [aws_eks_cluster.main]
}
