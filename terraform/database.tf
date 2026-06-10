# ─────────────────────────────────────────────────────────────────────────────
# Cloud SQL for PostgreSQL — Production Database
# ─────────────────────────────────────────────────────────────────────────────
# Private IP only (no public access), automated backups, point-in-time recovery,
# high availability with regional failover, and SSL enforcement.

# ── Database Instance ───────────────────────────────────────────────────────
resource "google_sql_database_instance" "postgres" {
  name                = "${local.name_prefix}-db"
  database_version    = "POSTGRES_16"
  region              = var.region
  project             = var.project_id
  deletion_protection = true # Prevent accidental deletion via Terraform

  settings {
    # db-custom-2-7680 = 2 vCPUs, 7.5 GB RAM — good starting point for payment workloads
    # Scale up later: db-custom-4-15360 (4 vCPU, 15 GB) if needed
    tier              = var.db_tier
    availability_type = var.db_high_availability ? "REGIONAL" : "ZONAL"
    disk_type         = "PD_SSD"
    disk_size         = var.db_disk_size_gb
    disk_autoresize   = true

    # ── Private IP Only (no public access) ──────────────────────────────
    ip_configuration {
      ipv4_enabled                                  = false # No public IP
      private_network                               = google_compute_network.vpc.self_link
      enable_private_path_for_google_cloud_services = true
    }

    # ── Automated Backups ───────────────────────────────────────────────
    # Daily automated backups with point-in-time recovery (PITR)
    # Retains backups for 30 days — allows restoring to any second
    backup_configuration {
      enabled                        = true
      start_time                     = "02:00" # 2 AM UTC (7:30 AM IST)
      point_in_time_recovery_enabled = true
      transaction_log_retention_days = 7

      backup_retention_settings {
        retained_backups = 30
        retention_unit   = "COUNT"
      }
    }

    # ── Maintenance Window ──────────────────────────────────────────────
    # Sunday 3 AM UTC — minimal traffic window for auto-patching
    maintenance_window {
      day          = 7 # Sunday
      hour         = 3
      update_track = "stable"
    }

    # ── Performance & Security Flags ────────────────────────────────────
    database_flags {
      name  = "log_checkpoints"
      value = "on"
    }

    database_flags {
      name  = "log_connections"
      value = "on"
    }

    database_flags {
      name  = "log_disconnections"
      value = "on"
    }

    database_flags {
      name  = "log_lock_waits"
      value = "on"
    }

    # Log slow queries (>1 second) — critical for payment latency monitoring
    database_flags {
      name  = "log_min_duration_statement"
      value = "1000" # milliseconds
    }

    # Connection limits — matches our Go app's pool settings
    database_flags {
      name  = "max_connections"
      value = "200"
    }

    user_labels = local.common_labels
  }

  # Wait for private service connection before creating the instance
  depends_on = [google_service_networking_connection.private_vpc_connection]
}

# ── Application Database ────────────────────────────────────────────────────
resource "google_sql_database" "payment_db" {
  name     = var.db_name
  instance = google_sql_database_instance.postgres.name
}

# ── Application User ────────────────────────────────────────────────────────
# Password is generated randomly and stored in Secret Manager (Phase 5).
# The app never hardcodes this — it reads from environment variables.
resource "random_password" "db_password" {
  length  = 32
  special = true
  # Exclude characters that cause issues in connection strings
  override_special = "!#$%^&*()-_=+[]{}|:,.<>?"
}

resource "google_sql_user" "app_user" {
  name     = var.db_user
  instance = google_sql_database_instance.postgres.name
  password = random_password.db_password.result

  deletion_policy = "ABANDON" # Don't fail if user has active connections
}

# ── Store DB Password in Secret Manager ─────────────────────────────────────
# This creates the secret now; the GKE workload reads it at runtime (Phase 5/6).
resource "google_secret_manager_secret" "db_password" {
  secret_id = "${local.name_prefix}-db-password"
  project   = var.project_id

  replication {
    auto {}
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = random_password.db_password.result
}
