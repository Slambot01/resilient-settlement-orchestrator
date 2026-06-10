# ─────────────────────────────────────────────────────────────────────────────
# Memorystore for Redis — Managed Redis Instance
# ─────────────────────────────────────────────────────────────────────────────
# Used by the payment orchestrator for:
#   - Idempotency key storage (prevents double-charging)
#   - Rate limiting (per-IP request throttling)
#   - Circuit breaker state
#
# Private IP only — accessible from GKE pods via the VPC.

resource "google_redis_instance" "cache" {
  name           = "${local.name_prefix}-redis"
  tier           = "STANDARD_HA" # High availability with automatic failover
  memory_size_gb = var.redis_memory_size_gb
  region         = var.region
  project        = var.project_id

  redis_version = var.redis_version

  # ── Network — Private IP within our VPC ───────────────────────────────
  authorized_network = google_compute_network.vpc.id
  connect_mode       = "PRIVATE_SERVICE_ACCESS"

  # ── Persistence — RDB snapshots for crash recovery ────────────────────
  # Without persistence, a Redis restart would lose all idempotency keys,
  # potentially allowing duplicate payments to go through.
  persistence_config {
    persistence_mode    = "RDB"
    rdb_snapshot_period = "ONE_HOUR"
  }

  # ── Maintenance Window ────────────────────────────────────────────────
  # Sunday 4 AM UTC — after Cloud SQL maintenance (3 AM)
  maintenance_policy {
    weekly_maintenance_window {
      day = "SUNDAY"
      start_time {
        hours   = 4
        minutes = 0
      }
    }
  }

  # ── Redis Configuration ───────────────────────────────────────────────
  redis_configs = {
    # Eviction policy: don't evict keys silently — fail loudly
    # For a payment system, losing an idempotency key is worse than an error
    maxmemory-policy = "noeviction"

    # Notify on key expiration events (useful for monitoring)
    notify-keyspace-events = "Ex"
  }

  labels = local.common_labels

  depends_on = [
    google_project_service.apis["redis.googleapis.com"],
    google_service_networking_connection.private_vpc_connection,
  ]

  lifecycle {
    prevent_destroy = true # Don't accidentally delete Redis with active idempotency data
  }
}

# ── Store Redis connection info in Secret Manager ───────────────────────────
# GKE pods will read these at runtime (Phase 6).

resource "google_secret_manager_secret" "redis_host" {
  secret_id = "${local.name_prefix}-redis-host"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret_version" "redis_host" {
  secret      = google_secret_manager_secret.redis_host.id
  secret_data = google_redis_instance.cache.host
}
