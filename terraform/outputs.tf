# ─────────────────────────────────────────────────────────────────────────────
# Outputs — Exported values used by later phases
# ─────────────────────────────────────────────────────────────────────────────

# ── Network ─────────────────────────────────────────────────────────────────

output "vpc_name" {
  description = "Name of the VPC network"
  value       = google_compute_network.vpc.name
}

output "vpc_self_link" {
  description = "Self-link of the VPC network (used by Cloud SQL, GKE)"
  value       = google_compute_network.vpc.self_link
}

output "gke_subnet_name" {
  description = "Name of the GKE subnet"
  value       = google_compute_subnetwork.gke.name
}

output "gke_subnet_self_link" {
  description = "Self-link of the GKE subnet"
  value       = google_compute_subnetwork.gke.self_link
}

output "gke_pods_range_name" {
  description = "Name of the secondary IP range for GKE pods"
  value       = google_compute_subnetwork.gke.secondary_ip_range[0].range_name
}

output "gke_services_range_name" {
  description = "Name of the secondary IP range for GKE services"
  value       = google_compute_subnetwork.gke.secondary_ip_range[1].range_name
}

output "db_subnet_name" {
  description = "Name of the database subnet"
  value       = google_compute_subnetwork.db.name
}

# ── Private Service Connection ──────────────────────────────────────────────

output "private_vpc_connection" {
  description = "Private VPC connection ID (required by Cloud SQL for private IP)"
  value       = google_service_networking_connection.private_vpc_connection.id
}

# ── Cloud NAT ───────────────────────────────────────────────────────────────

output "cloud_nat_name" {
  description = "Cloud NAT gateway name"
  value       = google_compute_router_nat.nat.name
}

# ── Metadata ────────────────────────────────────────────────────────────────

output "project_id" {
  description = "GCP project ID"
  value       = var.project_id
}

output "region" {
  description = "GCP region"
  value       = var.region
}

output "name_prefix" {
  description = "Resource naming prefix used across all phases"
  value       = local.name_prefix
}

# ── Cloud SQL (Phase 2) ────────────────────────────────────────────────────

output "db_instance_name" {
  description = "Cloud SQL instance name"
  value       = google_sql_database_instance.postgres.name
}

output "db_connection_name" {
  description = "Cloud SQL connection name (used by Cloud SQL Auth Proxy in GKE)"
  value       = google_sql_database_instance.postgres.connection_name
}

output "db_private_ip" {
  description = "Cloud SQL private IP address"
  value       = google_sql_database_instance.postgres.private_ip_address
}

output "db_name" {
  description = "PostgreSQL database name"
  value       = google_sql_database.payment_db.name
}

output "db_user" {
  description = "PostgreSQL application username"
  value       = google_sql_user.app_user.name
}

output "db_password_secret_id" {
  description = "Secret Manager secret ID for the database password"
  value       = google_secret_manager_secret.db_password.secret_id
}

# ── Memorystore Redis (Phase 3) ────────────────────────────────────────────

output "redis_host" {
  description = "Memorystore Redis private IP address"
  value       = google_redis_instance.cache.host
}

output "redis_port" {
  description = "Memorystore Redis port"
  value       = google_redis_instance.cache.port
}

output "redis_host_secret_id" {
  description = "Secret Manager secret ID for the Redis host"
  value       = google_secret_manager_secret.redis_host.secret_id
}

# ── Pub/Sub (Phase 4) ──────────────────────────────────────────────────────

output "pubsub_webhook_topic" {
  description = "Pub/Sub webhook events topic name"
  value       = google_pubsub_topic.webhook_events.name
}

output "pubsub_event_topic" {
  description = "Pub/Sub payment state changes topic name"
  value       = google_pubsub_topic.payment_events.name
}

output "pubsub_dlq_topic" {
  description = "Pub/Sub dead-letter topic name"
  value       = google_pubsub_topic.webhook_dlq.name
}

output "pubsub_webhook_subscription" {
  description = "Pub/Sub webhook processing subscription name"
  value       = google_pubsub_subscription.webhook_processor.name
}

output "pubsub_dlq_subscription" {
  description = "Pub/Sub DLQ reader subscription name"
  value       = google_pubsub_subscription.webhook_dlq_reader.name
}

output "pubsub_service_account_email" {
  description = "Service account email for Pub/Sub (used in Workload Identity binding)"
  value       = google_service_account.pubsub_sa.email
}

# ── Secret Manager & Service Account (Phase 5) ─────────────────────────────

output "app_service_account_email" {
  description = "Main application service account email (bind to GKE via Workload Identity)"
  value       = google_service_account.app_sa.email
}

output "secret_api_keys_id" {
  description = "Secret Manager ID for API keys"
  value       = google_secret_manager_secret.api_keys.secret_id
}

output "secret_stripe_key_id" {
  description = "Secret Manager ID for Stripe secret key"
  value       = google_secret_manager_secret.stripe_secret_key.secret_id
}

output "secret_stripe_webhook_id" {
  description = "Secret Manager ID for Stripe webhook secret"
  value       = google_secret_manager_secret.stripe_webhook_secret.secret_id
}

output "secret_razorpay_key_id" {
  description = "Secret Manager ID for Razorpay key ID"
  value       = google_secret_manager_secret.razorpay_key_id.secret_id
}

output "secret_razorpay_secret_id" {
  description = "Secret Manager ID for Razorpay key secret"
  value       = google_secret_manager_secret.razorpay_key_secret.secret_id
}

# ── GKE & Artifact Registry (Phase 6) ──────────────────────────────────────

output "gke_cluster_name" {
  description = "GKE cluster name"
  value       = google_container_cluster.primary.name
}

output "gke_cluster_endpoint" {
  description = "GKE cluster API endpoint"
  value       = google_container_cluster.primary.endpoint
  sensitive   = true
}

output "artifact_registry_url" {
  description = "Docker image registry URL for pushing/pulling images"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}"
}
