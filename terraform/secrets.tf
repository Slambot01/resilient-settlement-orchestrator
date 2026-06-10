# ─────────────────────────────────────────────────────────────────────────────
# Google Cloud Secret Manager — Centralized Secret Storage
# ─────────────────────────────────────────────────────────────────────────────
# Stores all application secrets (API keys, PSP credentials) in a secure,
# auditable, versioned secret store. GKE pods read these at runtime via
# environment variables injected from Kubernetes Secrets (synced from GSM).
#
# NOTE: DB password and Redis host secrets are already created in
# database.tf and redis.tf respectively. This file handles the remaining
# application secrets.

# ── API Keys Secret ─────────────────────────────────────────────────────────
# Merchant-scoped API keys for authenticating requests to the payment API.
# Format: "merchantID:apiKey" (comma-separated for multiple merchants)
resource "google_secret_manager_secret" "api_keys" {
  secret_id = "${local.name_prefix}-api-keys"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

# Placeholder version - replace with real keys before production deployment
resource "google_secret_manager_secret_version" "api_keys" {
  secret      = google_secret_manager_secret.api_keys.id
  secret_data = "merchant_default:CHANGE_ME_BEFORE_PRODUCTION"
}

# ── Stripe Credentials ──────────────────────────────────────────────────────
resource "google_secret_manager_secret" "stripe_secret_key" {
  secret_id = "${local.name_prefix}-stripe-secret-key"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret" "stripe_webhook_secret" {
  secret_id = "${local.name_prefix}-stripe-webhook-secret"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

# ── Razorpay Credentials ────────────────────────────────────────────────────
resource "google_secret_manager_secret" "razorpay_key_id" {
  secret_id = "${local.name_prefix}-razorpay-key-id"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret" "razorpay_key_secret" {
  secret_id = "${local.name_prefix}-razorpay-key-secret"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

# ── Application Service Account ─────────────────────────────────────────────
# Main service account for the payment orchestrator app running on GKE.
# This SA gets bound to Kubernetes pods via Workload Identity (Phase 6).
resource "google_service_account" "app_sa" {
  account_id   = "${var.app_name}-app"
  display_name = "Payment Orchestrator Application Service Account"
  project      = var.project_id
}

# ── IAM: Allow app SA to read all secrets ────────────────────────────────────
# Grant secretAccessor role on each secret individually (least-privilege).
locals {
  app_secrets = [
    google_secret_manager_secret.api_keys.secret_id,
    google_secret_manager_secret.stripe_secret_key.secret_id,
    google_secret_manager_secret.stripe_webhook_secret.secret_id,
    google_secret_manager_secret.razorpay_key_id.secret_id,
    google_secret_manager_secret.razorpay_key_secret.secret_id,
    google_secret_manager_secret.db_password.secret_id,
    google_secret_manager_secret.redis_host.secret_id,
  ]
}

resource "google_secret_manager_secret_iam_member" "app_secret_access" {
  for_each  = toset(local.app_secrets)
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.app_sa.email}"
  project   = var.project_id
}

# ── IAM: Allow app SA to use Pub/Sub ─────────────────────────────────────────
# The app SA also needs Pub/Sub permissions (consolidating from pubsub.tf SA)
resource "google_project_iam_member" "app_sa_pubsub" {
  project = var.project_id
  role    = "roles/pubsub.editor"
  member  = "serviceAccount:${google_service_account.app_sa.email}"
}

# ── IAM: Allow app SA to connect to Cloud SQL ────────────────────────────────
resource "google_project_iam_member" "app_sa_cloudsql" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.app_sa.email}"
}

# ── IAM: Allow app SA to write logs and metrics ──────────────────────────────
resource "google_project_iam_member" "app_sa_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.app_sa.email}"
}

resource "google_project_iam_member" "app_sa_monitoring" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.app_sa.email}"
}
