locals {
  common_labels = {
    "app.kubernetes.io/part-of"    = "txn-platform"
    "app.kubernetes.io/managed-by" = "terraform"
  }
}

resource "kubernetes_namespace_v1" "txn" {
  metadata {
    name   = var.namespace
    labels = local.common_labels
  }
}

resource "kubernetes_config_map_v1" "feature_flags" {
  metadata {
    name      = "txn-feature-flags"
    namespace = kubernetes_namespace_v1.txn.metadata.name
    labels    = local.common_labels
  }

  data = var.feature_flags
}
