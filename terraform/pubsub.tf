# ─────────────────────────────────────────────────────────────────────────────
# Google Cloud Pub/Sub — Topics, Subscriptions & IAM
# ─────────────────────────────────────────────────────────────────────────────
# The Go code for publishing and subscribing is already implemented in:
#   - internal/pubsub/pubsub.go (publisher + topic/subscription bootstrapping)
#   - internal/pubsub/subscriber.go (webhook processor + DLQ management)
#
# This Terraform config creates the PRODUCTION resources that replace
# the local Pub/Sub emulator used in docker-compose.yml.
#
# NOTE: The app code also creates topics/subscriptions idempotently on startup
# (ensureTopic/ensureSubscription). This Terraform definition ensures they
# exist before the app starts and have proper IAM bindings.

# ── Webhook Events Topic ────────────────────────────────────────────────────
# Incoming PSP webhooks (Stripe, Razorpay) are published here for async processing.
resource "google_pubsub_topic" "webhook_events" {
  name    = "webhook-events"
  project = var.project_id

  # Retain messages for 7 days (allows replaying if subscriber was down)
  message_retention_duration = "604800s" # 7 days

  labels = local.common_labels

  depends_on = [google_project_service.apis["pubsub.googleapis.com"]]
}

# ── Payment State Changes Topic ─────────────────────────────────────────────
# Published when a payment transitions state (e.g., authorized → captured).
# Downstream consumers (analytics, notifications, audit trail) subscribe here.
resource "google_pubsub_topic" "payment_events" {
  name    = "payment-state-changes"
  project = var.project_id

  message_retention_duration = "604800s" # 7 days

  labels = local.common_labels

  depends_on = [google_project_service.apis["pubsub.googleapis.com"]]
}

# ── Dead Letter Topic ───────────────────────────────────────────────────────
# Webhooks that fail processing after max retries land here.
# The admin DLQ API (internal/handler/dlq.go) reads from this for manual retry.
resource "google_pubsub_topic" "webhook_dlq" {
  name    = "webhook-events-dlq"
  project = var.project_id

  message_retention_duration = "604800s" # 7 days

  labels = local.common_labels

  depends_on = [google_project_service.apis["pubsub.googleapis.com"]]
}

# ── Webhook Processing Subscription ────────────────────────────────────────
# The app's StartWebhookSubscriber() pulls from this subscription.
# Failed messages retry with exponential backoff, then route to the DLQ.
resource "google_pubsub_subscription" "webhook_processor" {
  name    = "webhook-processor"
  topic   = google_pubsub_topic.webhook_events.id
  project = var.project_id

  # 60-second ack deadline — matches the app's processing timeout
  ack_deadline_seconds = 60

  # Retry with exponential backoff
  retry_policy {
    minimum_backoff = "1s"
    maximum_backoff = "60s"
  }

  # Dead-letter policy — after 5 failed attempts, move to DLQ
  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.webhook_dlq.id
    max_delivery_attempts = 5
  }

  # Keep unacked messages for 7 days before expiry
  message_retention_duration = "604800s"
  retain_acked_messages      = false

  # Auto-expire the subscription if unused for 31 days (safety net)
  expiration_policy {
    ttl = "2678400s" # 31 days
  }

  labels = local.common_labels
}

# ── DLQ Reader Subscription ────────────────────────────────────────────────
# The admin API's PullDLQEntries() reads from this to list/retry failed messages.
resource "google_pubsub_subscription" "webhook_dlq_reader" {
  name    = "webhook-dlq-reader"
  topic   = google_pubsub_topic.webhook_dlq.id
  project = var.project_id

  ack_deadline_seconds = 60

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = "2678400s" # 31 days
  }

  labels = local.common_labels
}

# ── Service Account for Pub/Sub ─────────────────────────────────────────────
# Dedicated service account for the payment app to publish/subscribe.
# Bound to GKE pods via Workload Identity (Phase 6).
resource "google_service_account" "pubsub_sa" {
  account_id   = "${var.app_name}-pubsub"
  display_name = "Payment Orchestrator Pub/Sub Service Account"
  project      = var.project_id
}

# ── IAM — Publish to webhook and payment event topics ───────────────────────
resource "google_pubsub_topic_iam_member" "webhook_publisher" {
  topic   = google_pubsub_topic.webhook_events.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.pubsub_sa.email}"
  project = var.project_id
}

resource "google_pubsub_topic_iam_member" "event_publisher" {
  topic   = google_pubsub_topic.payment_events.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.pubsub_sa.email}"
  project = var.project_id
}

# Republishing from DLQ back to webhook topic (RetryDLQMessage)
resource "google_pubsub_topic_iam_member" "dlq_republisher" {
  topic   = google_pubsub_topic.webhook_events.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.pubsub_sa.email}"
  project = var.project_id
}

# ── IAM — Subscribe to webhook and DLQ subscriptions ────────────────────────
resource "google_pubsub_subscription_iam_member" "webhook_subscriber" {
  subscription = google_pubsub_subscription.webhook_processor.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:${google_service_account.pubsub_sa.email}"
  project      = var.project_id
}

resource "google_pubsub_subscription_iam_member" "dlq_subscriber" {
  subscription = google_pubsub_subscription.webhook_dlq_reader.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:${google_service_account.pubsub_sa.email}"
  project      = var.project_id
}

# ── IAM — Allow Pub/Sub system to forward messages to DLQ ───────────────────
# The Pub/Sub service agent needs publish rights on the DLQ topic
# and subscriber rights on the source subscription to forward dead-lettered messages.
data "google_project" "project" {
  project_id = var.project_id
}

resource "google_pubsub_topic_iam_member" "dlq_pubsub_agent" {
  topic   = google_pubsub_topic.webhook_dlq.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
  project = var.project_id
}

resource "google_pubsub_subscription_iam_member" "webhook_pubsub_agent" {
  subscription = google_pubsub_subscription.webhook_processor.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
  project      = var.project_id
}
