# ─────────────────────────────────────────────────────────────────────────────
# VPC Network, Subnets, Cloud Router, Cloud NAT & Private Service Connection
# ─────────────────────────────────────────────────────────────────────────────

# ── VPC Network ─────────────────────────────────────────────────────────────
# Custom-mode VPC — we define subnets explicitly (no auto-created ones)
resource "google_compute_network" "vpc" {
  name                    = "${local.name_prefix}-vpc"
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
  project                 = var.project_id

  depends_on = [google_project_service.apis["compute.googleapis.com"]]
}

# ── GKE Subnet ──────────────────────────────────────────────────────────────
# Primary range for node IPs, secondary ranges for pod and service IPs.
# GKE requires separate IP ranges for pods and services (VPC-native mode).
resource "google_compute_subnetwork" "gke" {
  name          = "${local.name_prefix}-gke-subnet"
  ip_cidr_range = var.gke_subnet_cidr
  region        = var.region
  network       = google_compute_network.vpc.id

  # Enable private Google API access — allows nodes to reach Google APIs
  # (Pub/Sub, Secret Manager, etc.) without public IPs
  private_ip_google_access = true

  # GKE pod and service secondary ranges (VPC-native cluster requirement)
  secondary_ip_range {
    range_name    = "${local.name_prefix}-pods"
    ip_cidr_range = var.gke_pods_cidr
  }

  secondary_ip_range {
    range_name    = "${local.name_prefix}-services"
    ip_cidr_range = var.gke_services_cidr
  }

  # Enable VPC Flow Logs for network debugging and security auditing
  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }
}

# ── Database Subnet ─────────────────────────────────────────────────────────
# Dedicated subnet for Cloud SQL and Memorystore private IPs.
# Keeping DB traffic on a separate subnet improves security isolation.
resource "google_compute_subnetwork" "db" {
  name          = "${local.name_prefix}-db-subnet"
  ip_cidr_range = var.db_subnet_cidr
  region        = var.region
  network       = google_compute_network.vpc.id

  private_ip_google_access = true
}

# ── Private Service Connection ──────────────────────────────────────────────
# Required for Cloud SQL and Memorystore to get private IPs within our VPC.
# This allocates a CIDR range to Google-managed services (peered VPC).

resource "google_compute_global_address" "private_ip_range" {
  name          = "${local.name_prefix}-private-ip-range"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  address       = split("/", var.private_service_cidr)[0] # "10.2.0.0"
  network       = google_compute_network.vpc.id
}

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_range.name]

  depends_on = [google_project_service.apis["servicenetworking.googleapis.com"]]
}

# ── Cloud Router ────────────────────────────────────────────────────────────
# Required by Cloud NAT — routes outbound traffic from private GKE nodes.
resource "google_compute_router" "router" {
  name    = "${local.name_prefix}-router"
  region  = var.region
  network = google_compute_network.vpc.id

  bgp {
    asn = 64514 # Private ASN for Cloud Router
  }
}

# ── Cloud NAT ───────────────────────────────────────────────────────────────
# Enables outbound internet for GKE pods that have no public IPs.
# Critical for: calling Stripe/Razorpay APIs, pulling container images,
# downloading Go dependencies during builds.
#
# Uses auto-allocated NAT IPs — GCP manages IP rotation.
# The NAT IPs can be whitelisted with PSPs if they require IP allowlisting.
resource "google_compute_router_nat" "nat" {
  name   = "${local.name_prefix}-nat"
  router = google_compute_router.router.name
  region = var.region

  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  # Only NAT traffic from the GKE subnet — DB subnet doesn't need internet
  subnetwork {
    name                    = google_compute_subnetwork.gke.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }

  # Logging for debugging NAT issues (e.g., port exhaustion)
  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}
