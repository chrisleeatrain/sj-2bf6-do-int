variable "kubeconfig_path" {
  description = "Path to the kubeconfig file."
  type        = string
  default     = "~/.kube/config"
}

variable "kube_context" {
  description = "kubeconfig context to target. kind names it kind-<cluster name>."
  type        = string
  default     = "kind-txn-sandbox"
}

variable "namespace" {
  description = "Namespace the txn-api runs in."
  type        = string
  default     = "txn-platform"
}

variable "feature_flags" {
  description = "Feature flags published to the txn-feature-flags ConfigMap."
  type        = map(string)
  default = {
    "instant settlement" = "false"
    "usdc-payouts"       = "true"
  }
}
