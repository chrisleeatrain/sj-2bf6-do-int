# Credentials for the ledger integration. The app will read these from the
# txn-ledger-credentials Secret once the ledger client lands.

resource "random_password" "ledger_api_key" {
  length  = 32
  special = false
}

resource "kubernetes_secret_v1" "ledger_credentials" {
  metadata {
    name      = "txn-ledger-credentials"
    namespace = kubernetes_namespace_v1.txn.metadata[0].name
    labels    = local.common_labels
  }

  data = {
    LEDGER_API_KEY = random_password.ledger_api_key.result
  }
}

output "ledger_api_key" {
  description = "API key for the ledger. Marked sensitive so it doesn't show up in CI logs."
  value       = random_password.ledger_api_key.result
  sensitive   = true
}
