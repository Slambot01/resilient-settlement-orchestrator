# Resilient Settlement Orchestrator Architecture

## Overview

The Resilient Settlement Orchestrator is a robust payment orchestration and ledger system designed for high availability, transactional integrity, and extensibility. It acts as an abstraction layer between merchant applications and multiple Payment Service Providers (PSPs).

## Core Components

### 1. API Layer (go-chi)
Provides RESTful endpoints for merchant integration.
- Includes API key authentication.
- Implements Idempotency (via Redis) to prevent duplicate transactions.
- Uses structured logging and captures Prometheus metrics.

### 2. Payment Orchestration Service
The central brain for processing payments.
- Manages the payment state machine (Created -> Processing -> Captured / Failed).
- Uses a rules-based routing engine to select the best PSP based on cost, currency, and availability.

### 3. PSP Adapters
Standardized interfaces to communicate with external gateways.
- Contains adapters for Stripe, Razorpay, and a Mock PSP.
- Wrapped in Circuit Breakers to handle upstream failures gracefully.
- Employs exponential backoff retries for transient errors.

### 4. Double-Entry Ledger
Financial-grade bookkeeping system.
- Ensures all money movements are recorded as balanced debits and credits.
- Uses PostgreSQL with ACID transactions and row-level locking to prevent race conditions.

### 5. Webhook Processor & Reconciliation Engine
Handles asynchronous updates from PSPs.
- Verifies HMAC signatures for security.
- Reconciles expected payment states with actual settled states, generating discrepancy reports.
- Uses a Dead Letter Queue (DLQ) for failed webhooks.

## Data Flow
1. Client requests payment creation via REST API.
2. Idempotency middleware checks for duplicate requests.
3. Payment service creates a local record and selects a PSP via the Router.
4. PSP Adapter attempts the API call, protected by a circuit breaker and retry logic.
5. On success, the Ledger service records the transaction.
6. Asynchronous webhooks later confirm settlement, processed securely via the Webhook engine.

## Infrastructure Setup
- **App**: Go 1.25+ compiled to a single binary.
- **Database**: PostgreSQL 16 for persistence.
- **Cache/Queue**: Redis 7 for idempotency and DLQ.
- **Containerization**: Docker and Docker Compose for easy deployment.
