{{/*
Expand the name of the chart.
*/}}
{{- define "litellm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "litellm.fullname" -}}
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
{{- define "litellm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "litellm.labels" -}}
helm.sh/chart: {{ include "litellm.chart" . }}
{{ include "litellm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "litellm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "litellm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "litellm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "litellm.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Proxy full name
*/}}
{{- define "litellm.proxy.fullname" -}}
{{- printf "%s-proxy" (include "litellm.fullname" .) }}
{{- end }}

{{/*
Create environment variables
*/}}
{{- define "litellm.proxy.env" -}}
{{- $env := list }}
{{- if .Values.proxy.masterKey }}
{{- $env = append $env (dict "name" "LITELLM_MASTER_KEY" "valueFrom" (dict "secretKeyRef" (dict "name" "litellm-secrets" "key" "LITELLM_MASTER_KEY"))) }}
{{- end }}
{{- $env = append $env (dict "name" "LITELLM_CONFIG" "value" "/etc/litellm/config.yaml") }}
{{- $env = append $env (dict "name" "OTEL_SERVICE_NAME" "value" "litellm-proxy") }}
{{- if .Values.proxy.env }}
{{- range $k, $v := .Values.proxy.env }}
{{- $env = append $env (dict "name" $k "value" $v) }}
{{- end }}
{{- end }}
{{- return $env }}
{{- end }}
