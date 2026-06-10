# ─────────────────────────────────────────────────────────────────────────────
# Google Cloud Monitoring - Alerts, Uptime Checks & Dashboard
# ─────────────────────────────────────────────────────────────────────────────
# Integrates with the structured JSON logs (slog) and Prometheus metrics
# already built into the Go application.

# ── Notification Channel (Email) ────────────────────────────────────────────
# Alerts are sent here. Add Slack/PagerDuty channels as needed.
resource "google_monitoring_notification_channel" "email" {
  display_name = "${local.name_prefix} Alert Email"
  type         = "email"
  project      = var.project_id

  labels = {
    email_address = var.alert_email
  }

  depends_on = [google_project_service.apis["monitoring.googleapis.com"]]
}

# ── Uptime Check - External Health Probe ────────────────────────────────────
# Google Cloud probes /healthz every 60 seconds from multiple global locations.
# Alerts if the endpoint is down for 2 consecutive checks.
resource "google_monitoring_uptime_check_config" "health" {
  display_name = "${local.name_prefix} Health Check"
  project      = var.project_id
  timeout      = "10s"
  period       = "60s"

  http_check {
    path         = "/healthz"
    port         = 443
    use_ssl      = true
    validate_ssl = true
  }

  # Uses the Ingress external IP or domain
  monitored_resource {
    type = "uptime_url"
    labels = {
      project_id = var.project_id
      host       = var.app_domain
    }
  }

  depends_on = [google_project_service.apis["monitoring.googleapis.com"]]
}

# ── Alert: High Error Rate ──────────────────────────────────────────────────
# Fires when HTTP 5xx error rate exceeds 5% over 5 minutes.
# Critical for a payment system - indicates broken transactions.
resource "google_monitoring_alert_policy" "high_error_rate" {
  display_name = "${local.name_prefix} - High Error Rate (>5%)"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "HTTP 5xx rate > 5%"

    condition_threshold {
      filter = <<-EOT
        resource.type = "k8s_container"
        AND resource.labels.namespace_name = "payment-system"
        AND metric.type = "logging.googleapis.com/user/http_5xx_count"
      EOT

      comparison      = "COMPARISON_GT"
      threshold_value = 5
      duration        = "300s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]

  alert_strategy {
    auto_close = "1800s" # Auto-resolve after 30 min of no errors
  }

  documentation {
    content   = "High HTTP 5xx error rate detected in the payment orchestrator. Check Cloud Logging for error details: `resource.type=\"k8s_container\" AND resource.labels.namespace_name=\"payment-system\" AND severity>=ERROR`"
    mime_type = "text/markdown"
  }

  depends_on = [google_project_service.apis["monitoring.googleapis.com"]]
}

# ── Alert: High Latency ────────────────────────────────────────────────────
# Fires when P95 response time exceeds 2 seconds.
# Payment APIs should respond within 1-2 seconds max.
resource "google_monitoring_alert_policy" "high_latency" {
  display_name = "${local.name_prefix} - High Latency (P95 > 2s)"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "Request latency P95 > 2000ms"

    condition_threshold {
      filter = <<-EOT
        resource.type = "k8s_container"
        AND resource.labels.namespace_name = "payment-system"
        AND metric.type = "logging.googleapis.com/user/http_request_duration_seconds"
      EOT

      comparison      = "COMPARISON_GT"
      threshold_value = 2.0
      duration        = "300s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_PERCENTILE_95"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]

  alert_strategy {
    auto_close = "1800s"
  }

  documentation {
    content   = "P95 latency exceeded 2 seconds. Possible causes: database slowdown (check Cloud SQL metrics), PSP API timeouts (check circuit breaker logs), or high pod CPU usage (check HPA scaling)."
    mime_type = "text/markdown"
  }

  depends_on = [google_project_service.apis["monitoring.googleapis.com"]]
}

# ── Alert: Pod Restart Loop ─────────────────────────────────────────────────
# Fires when a pod restarts more than 3 times in 10 minutes.
# Indicates a crash loop - likely a code bug or config issue.
resource "google_monitoring_alert_policy" "pod_restarts" {
  display_name = "${local.name_prefix} - Pod Crash Loop"
  project      = var.project_id
  combiner     = "OR"

  conditions {
    display_name = "Container restart count > 3 in 10 min"

    condition_threshold {
      filter = <<-EOT
        resource.type = "k8s_container"
        AND resource.labels.namespace_name = "payment-system"
        AND metric.type = "kubernetes.io/container/restart_count"
      EOT

      comparison      = "COMPARISON_GT"
      threshold_value = 3
      duration        = "600s"

      aggregations {
        alignment_period   = "300s"
        per_series_aligner = "ALIGN_DELTA"
      }
    }
  }

  notification_channels = [google_monitoring_notification_channel.email.name]

  documentation {
    content   = "Pod is crash-looping. Check logs: `kubectl logs -n payment-system -l app=payment-orchestrator --previous`"
    mime_type = "text/markdown"
  }

  depends_on = [google_project_service.apis["monitoring.googleapis.com"]]
}

# ── Log-Based Metric: Payment Failures ──────────────────────────────────────
# Counts payment processing errors from structured logs.
# Used by dashboards and alert policies.
resource "google_logging_metric" "payment_errors" {
  name    = "payment-processing-errors"
  project = var.project_id
  filter  = <<-EOT
    resource.type = "k8s_container"
    AND resource.labels.namespace_name = "payment-system"
    AND severity >= ERROR
    AND jsonPayload.msg =~ "payment.*fail"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "psp"
      value_type  = "STRING"
      description = "Payment Service Provider"
    }
  }

  label_extractors = {
    "psp" = "EXTRACT(jsonPayload.psp)"
  }

  depends_on = [google_project_service.apis["logging.googleapis.com"]]
}
