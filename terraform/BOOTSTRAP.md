# GCP Bootstrap — First-Time Setup

> Run these steps **once** before `terraform init`. You need a Google account with billing enabled.

## Step 1: Install Prerequisites

```bash
# Install Google Cloud CLI
# https://cloud.google.com/sdk/docs/install

# Install Terraform
# https://developer.hashicorp.com/terraform/install

# Verify installations
gcloud --version
terraform --version
```

## Step 2: Create GCP Project

```bash
# Login to Google Cloud
gcloud auth login
gcloud auth application-default login

# Create a new project (pick a globally unique ID)
export PROJECT_ID="resilient-settlement-prod"
gcloud projects create $PROJECT_ID --name="Payment Orchestrator"

# Set it as the active project
gcloud config set project $PROJECT_ID

# Link a billing account (required for any resource creation)
# List available billing accounts:
gcloud billing accounts list
# Link billing (replace BILLING_ACCOUNT_ID with yours):
gcloud billing projects link $PROJECT_ID --billing-account=BILLING_ACCOUNT_ID
```

## Step 3: Create Terraform State Bucket

```bash
# Create a GCS bucket for Terraform remote state
# Bucket name must be globally unique — use your project ID as prefix
gsutil mb -p $PROJECT_ID -l asia-south1 -b on gs://${PROJECT_ID}-terraform-state

# Enable versioning (protects against accidental state deletion)
gsutil versioning set on gs://${PROJECT_ID}-terraform-state
```

## Step 4: Initialize Terraform

```bash
cd terraform/

# Copy the example tfvars and fill in your project ID
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your project_id

# Initialize Terraform (downloads providers, connects to GCS backend)
terraform init

# Preview what will be created
terraform plan

# Apply the infrastructure
terraform apply
```

## What Gets Created (Phase 1)

| Resource | Purpose | Cost |
|---|---|---|
| GCP APIs (10+) | Enable Compute, GKE, SQL, Pub/Sub, etc. | Free |
| VPC Network | Private network for all resources | Free |
| GKE Subnet | Node IPs + pod/service secondary ranges | Free |
| DB Subnet | Private IPs for Cloud SQL & Redis | Free |
| Cloud Router | Routes traffic for NAT gateway | Free |
| Cloud NAT | Outbound internet for private nodes (Stripe/Razorpay API calls) | ~$1/day |
| Firewall Rules | Allow health checks, internal traffic | Free |
| Private Service Connection | Private IP access to Cloud SQL/Redis | Free |

**Total Phase 1 cost: ~$1/day** (Cloud NAT only)
