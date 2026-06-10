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
