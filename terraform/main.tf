# ─────────────────────────────────────────────────────────────────────────────
# Terraform Configuration — Provider, Backend & API Enablement
# ─────────────────────────────────────────────────────────────────────────────

terraform {
  required_version = ">= 1.5.0"

  # Remote state stored in GCS — see BOOTSTRAP.md for bucket creation
  backend "gcs" {
    bucket = "resilient-settlement-prod-terraform-state"
    prefix = "terraform/state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

# ── Provider Configuration ──────────────────────────────────────────────────

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

# ── Local Values ────────────────────────────────────────────────────────────

locals {
  # Consistent resource naming: payment-orchestrator-production
  name_prefix = "${var.app_name}-${var.environment}"

  # Common labels applied to all resources for cost tracking and organization
  common_labels = {
    app         = var.app_name
    environment = var.environment
    managed_by  = "terraform"
    project     = "resilient-settlement-orchestrator"
  }
}

# ── Enable Required GCP APIs ────────────────────────────────────────────────
# These must be enabled before any resources can be created.
# Some take 30-60 seconds to propagate after enablement.

resource "google_project_service" "apis" {
  for_each = toset([
    "compute.googleapis.com",              # VPC, firewall, load balancers
    "container.googleapis.com",            # Google Kubernetes Engine (Phase 6)
    "sqladmin.googleapis.com",             # Cloud SQL for PostgreSQL (Phase 2)
    "redis.googleapis.com",                # Memorystore for Redis (Phase 3)
    "pubsub.googleapis.com",               # Google Cloud Pub/Sub (Phase 4)
    "secretmanager.googleapis.com",        # Secret Manager (Phase 5)
    "artifactregistry.googleapis.com",     # Docker image registry (Phase 7)
    "cloudresourcemanager.googleapis.com", # Project-level IAM
    "iam.googleapis.com",                  # Service accounts, Workload Identity
    "servicenetworking.googleapis.com",    # Private service connections (Cloud SQL/Redis private IP)
    "logging.googleapis.com",              # Cloud Logging (Phase 8)
    "monitoring.googleapis.com",           # Cloud Monitoring (Phase 8)
  ])

  project = var.project_id
  service = each.value

  # Don't disable the API if this resource is destroyed — prevents accidental breakage
  disable_on_destroy = false
}
