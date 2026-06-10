# ─────────────────────────────────────────────────────────────────────────────
# Google Kubernetes Engine (GKE) - Production Cluster
# ─────────────────────────────────────────────────────────────────────────────
# Private cluster with Workload Identity, VPC-native networking,
# and auto-scaling node pools.

# ── GKE Cluster ─────────────────────────────────────────────────────────────
resource "google_container_cluster" "primary" {
  name     = "${local.name_prefix}-gke"
  location = var.region # Regional cluster (multi-zone HA)
  project  = var.project_id

  # We manage node pools separately for flexibility
  remove_default_node_pool = true
  initial_node_count       = 1

  # ── VPC-Native Networking ─────────────────────────────────────────────
  network    = google_compute_network.vpc.name
  subnetwork = google_compute_subnetwork.gke.name

  ip_allocation_policy {
    cluster_secondary_range_name  = google_compute_subnetwork.gke.secondary_ip_range[0].range_name
    services_secondary_range_name = google_compute_subnetwork.gke.secondary_ip_range[1].range_name
  }

  # ── Private Cluster ───────────────────────────────────────────────────
  # Nodes have no public IPs. Outbound internet via Cloud NAT (Phase 1).
  # Master is accessible from authorized networks only.
  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false # Allow kubectl from authorized networks
    master_ipv4_cidr_block  = "172.16.0.0/28"
  }

  # ── Workload Identity ─────────────────────────────────────────────────
  # Allows K8s service accounts to act as GCP service accounts.
  # This is how pods authenticate to Pub/Sub, Secret Manager, Cloud SQL
  # without needing JSON key files.
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  # ── Security ──────────────────────────────────────────────────────────
  # Binary Authorization could be added here for supply chain security
  release_channel {
    channel = "STABLE"
  }

  # ── Maintenance Window ────────────────────────────────────────────────
  maintenance_policy {
    recurring_window {
      start_time = "2024-01-01T02:00:00Z"
      end_time   = "2024-01-01T06:00:00Z"
      recurrence = "FREQ=WEEKLY;BYDAY=SU"
    }
  }

  # ── Monitoring & Logging ──────────────────────────────────────────────
  logging_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }

  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
    managed_prometheus {
      enabled = true
    }
  }

  # ── Network Policy ───────────────────────────────────────────────────
  network_policy {
    enabled = true
  }

  # ── Addons ────────────────────────────────────────────────────────────
  addons_config {
    http_load_balancing {
      disabled = false # GCP Ingress controller
    }
    horizontal_pod_autoscaling {
      disabled = false
    }
    network_policy_config {
      disabled = false
    }
  }

  resource_labels = local.common_labels

  depends_on = [
    google_project_service.apis["container.googleapis.com"],
    google_compute_subnetwork.gke,
  ]
}

# ── Node Pool ───────────────────────────────────────────────────────────────
# Separate node pool for application workloads with auto-scaling.
resource "google_container_node_pool" "app_nodes" {
  name     = "${local.name_prefix}-app-pool"
  location = var.region
  cluster  = google_container_cluster.primary.name
  project  = var.project_id

  # Auto-scaling: 1-5 nodes per zone (3 zones = 3-15 nodes total)
  autoscaling {
    min_node_count = 1
    max_node_count = 5
  }

  # Start with 1 node per zone
  initial_node_count = 1

  management {
    auto_repair  = true
    auto_upgrade = true
  }

  node_config {
    # e2-medium: 2 vCPU, 4 GB RAM - cost-effective for payment workloads
    machine_type = var.gke_node_machine_type
    disk_size_gb = 50
    disk_type    = "pd-standard"

    # Use the app service account (not the default compute SA)
    service_account = google_service_account.app_sa.email
    oauth_scopes    = ["https://www.googleapis.com/auth/cloud-platform"]

    # Workload Identity on nodes
    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    # Security: use Container-Optimized OS
    image_type = "COS_CONTAINERD"

    # Shielded VM for secure boot
    shielded_instance_config {
      enable_secure_boot          = true
      enable_integrity_monitoring = true
    }

    # Tags for firewall rules
    tags = ["gke-node"]

    labels = local.common_labels

    metadata = {
      disable-legacy-endpoints = "true"
    }
  }
}

# ── Workload Identity Binding ───────────────────────────────────────────────
# Binds the GCP service account to the Kubernetes service account.
# This allows pods running as "payment-app" K8s SA to authenticate
# as the GCP app SA automatically.
resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.app_sa.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[payment-system/payment-app]"
}

# ── Artifact Registry ──────────────────────────────────────────────────────
# Docker image repository for the payment orchestrator container.
resource "google_artifact_registry_repository" "app" {
  location      = var.region
  repository_id = "${var.app_name}-images"
  description   = "Docker images for the payment orchestrator"
  format        = "DOCKER"
  project       = var.project_id

  cleanup_policies {
    id     = "keep-recent"
    action = "KEEP"

    most_recent_versions {
      keep_count = 10
    }
  }

  labels = local.common_labels

  depends_on = [google_project_service.apis["artifactregistry.googleapis.com"]]
}
