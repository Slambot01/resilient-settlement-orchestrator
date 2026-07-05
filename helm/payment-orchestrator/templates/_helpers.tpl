{{/*
─────────────────────────────────────────────────────────────────────────────
Payment Orchestrator — Helm Template Helpers
─────────────────────────────────────────────────────────────────────────────
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "payment-orchestrator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited.
*/}}
{{- define "payment-orchestrator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version for the chart label.
*/}}
{{- define "payment-orchestrator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "payment-orchestrator.labels" -}}
helm.sh/chart: {{ include "payment-orchestrator.chart" . }}
{{ include "payment-orchestrator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels — used in matchLabels for Deployment/Service/HPA.
*/}}
{{- define "payment-orchestrator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "payment-orchestrator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: payment-orchestrator
{{- end }}

{{/*
Service account name.
*/}}
{{- define "payment-orchestrator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "payment-orchestrator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Namespace — always uses Release namespace.
*/}}
{{- define "payment-orchestrator.namespace" -}}
{{- .Release.Namespace }}
{{- end }}
