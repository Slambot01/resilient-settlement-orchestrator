# ─────────────────────────────────────────────────────────────────────────────
# Variables for the Payment Orchestrator GCP Infrastructure
# ─────────────────────────────────────────────────────────────────────────────

variable "project_id" {
  description = "GCP project ID where all resources will be created"
  type        = string

  validation {
    condition     = length(var.project_id) > 0
    error_message = "project_id must be set. See terraform.tfvars.example."
  }
}

variable "region" {
  description = "GCP region for resource deployment"
  type        = string
  default     = "asia-south1" # Mumbai — low latency for Indian users
}

variable "zone" {
  description = "GCP zone within the region"
  type        = string
  default     = "asia-south1-a"
}

variable "environment" {
  description = "Deployment environment (development, staging, production)"
  type        = string
  default     = "production"

  validation {
    condition     = contains(["development", "staging", "production"], var.environment)
    error_message = "environment must be one of: development, staging, production"
  }
}

variable "app_name" {
  description = "Application name used as prefix for all GCP resources"
  type        = string
  default     = "payment-orchestrator"
}

# ── Network CIDR Ranges ─────────────────────────────────────────────────────

variable "gke_subnet_cidr" {
  description = "Primary CIDR for GKE node IPs"
  type        = string
  default     = "10.0.0.0/20" # 4094 node IPs
}

variable "gke_pods_cidr" {
  description = "Secondary CIDR for GKE pod IPs"
  type        = string
  default     = "10.4.0.0/14" # ~262k pod IPs (GKE requirement)
}

variable "gke_services_cidr" {
  description = "Secondary CIDR for GKE service IPs"
  type        = string
  default     = "10.8.0.0/20" # 4094 service IPs
}

variable "db_subnet_cidr" {
  description = "CIDR for database subnet (Cloud SQL, Memorystore)"
  type        = string
  default     = "10.1.0.0/24" # 254 IPs — more than enough for managed DBs
}

variable "private_service_cidr" {
  description = "CIDR for Google private service connections (Cloud SQL private IP)"
  type        = string
  default     = "10.2.0.0/16" # Reserved for Google-managed services
}

# ── Cloud SQL (Phase 2) ────────────────────────────────────────────────────

variable "db_tier" {
  description = "Cloud SQL machine tier (vCPUs + RAM)"
  type        = string
  default     = "db-custom-2-7680" # 2 vCPU, 7.5 GB RAM
}

variable "db_disk_size_gb" {
  description = "Initial disk size in GB (auto-resizes)"
  type        = number
  default     = 20
}

variable "db_high_availability" {
  description = "Enable regional HA with automatic failover (doubles cost)"
  type        = bool
  default     = true
}

variable "db_name" {
  description = "PostgreSQL database name"
  type        = string
  default     = "payment_orchestrator"
}

variable "db_user" {
  description = "PostgreSQL application username"
  type        = string
  default     = "payment_user"
}

# ── Memorystore Redis (Phase 3) ────────────────────────────────────────────

variable "redis_memory_size_gb" {
  description = "Redis instance memory in GB"
  type        = number
  default     = 1
}

variable "redis_version" {
  description = "Redis version for Memorystore"
  type        = string
  default     = "REDIS_7_0"
}

# ── GKE (Phase 6) ──────────────────────────────────────────────────────────

variable "gke_node_machine_type" {
  description = "Machine type for GKE worker nodes"
  type        = string
  default     = "e2-medium" # 2 vCPU, 4 GB RAM - cost-effective
}
