output "namespace" {
  description = "Namespace the txn-api runs in."
  value       = kubernetes_namespace_v1.txn.metadata[0].name
}

output "feature_flags_config_map" {
  description = "Name of the feature flag ConfigMap."
  value       = kubernetes_config_map_v1.feature_flags.metadata[0].name
}
