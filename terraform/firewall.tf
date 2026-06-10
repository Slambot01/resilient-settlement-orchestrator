# ─────────────────────────────────────────────────────────────────────────────
# Firewall Rules
# ─────────────────────────────────────────────────────────────────────────────
# GCP firewall rules are stateful — return traffic is automatically allowed.
# Default behavior: all ingress is DENIED, all egress is ALLOWED.

# ── Allow Internal Traffic ──────────────────────────────────────────────────
# Permits all communication between resources within the VPC.
# Required for: GKE nodes ↔ pods, pods ↔ Cloud SQL, pods ↔ Redis.
resource "google_compute_firewall" "allow_internal" {
  name    = "${local.name_prefix}-allow-internal"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
  }

  allow {
    protocol = "udp"
  }

  allow {
    protocol = "icmp"
  }

  # All internal CIDR ranges
  source_ranges = [
    var.gke_subnet_cidr,
    var.gke_pods_cidr,
    var.gke_services_cidr,
    var.db_subnet_cidr,
  ]

  priority = 1000
}

# ── Allow GCP Health Check Probes ───────────────────────────────────────────
# These CIDR ranges are used by:
# - GKE Ingress health checks (for /healthz and /readyz)
# - Cloud Load Balancer health probes
# - Cloud NAT health checks
#
# NOTE: These exact CIDRs are also configured in our Go middleware
# (internal/middleware/realip.go) as trusted proxy ranges.
resource "google_compute_firewall" "allow_health_checks" {
  name    = "${local.name_prefix}-allow-health-checks"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["8080"] # Our app port
  }

  # Official GCP health check source ranges
  # Matches realip.go trusted proxy CIDRs
  source_ranges = [
    "35.191.0.0/16",  # GCP health check probes
    "130.211.0.0/22", # GCP load balancer health checks
  ]

  target_tags = ["gke-node"]

  priority = 900
}

# ── Allow GKE Master to Node Communication ──────────────────────────────────
# Required for kubectl exec, logs, port-forwarding, and webhook admission.
# GKE master communicates with nodes on port 443 (kubelet) and 10250 (metrics).
resource "google_compute_firewall" "allow_gke_master" {
  name    = "${local.name_prefix}-allow-gke-master"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["443", "10250"]
  }

  # GKE master CIDR is assigned dynamically — use the GKE-managed tag
  # The actual source range will be the master's private CIDR (set in Phase 6)
  source_ranges = ["172.16.0.0/28"] # Default GKE master CIDR

  target_tags = ["gke-node"]

  priority = 900
}
