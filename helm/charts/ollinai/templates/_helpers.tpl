{{/*
OllinAI Helm Chart - Template Helpers
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "titanops.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "titanops.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "titanops.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Component-specific labels.
Usage: {{ include "titanops.componentLabels" (dict "component" "change-intelligence" "context" .) }}
*/}}
{{- define "titanops.componentLabels" -}}
helm.sh/chart: {{ include "titanops.chart" .context }}
app.kubernetes.io/name: {{ include "titanops.name" .context }}
app.kubernetes.io/instance: {{ .context.Release.Name }}
app.kubernetes.io/version: {{ .context.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .context.Release.Service }}
app.kubernetes.io/part-of: titanops
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Component-specific selector labels.
Usage: {{ include "titanops.componentSelectorLabels" (dict "component" "change-intelligence" "context" .) }}
*/}}
{{- define "titanops.componentSelectorLabels" -}}
app.kubernetes.io/name: {{ include "titanops.name" .context }}
app.kubernetes.io/instance: {{ .context.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "titanops.serviceAccountName" -}}
{{- printf "%s-sa" (include "titanops.fullname" .) }}
{{- end }}

{{/*
Namespace for deployment.
*/}}
{{- define "titanops.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride }}
{{- end }}
