{{/*
Expand the name of the chart.
*/}}
{{- define "inferenceProxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "inferenceProxy.fullname" -}}
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
{{- define "inferenceProxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "inferenceProxy.labels" -}}
helm.sh/chart: {{ include "inferenceProxy.chart" . }}
{{ include "inferenceProxy.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "inferenceProxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "inferenceProxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Proxy full name
*/}}
{{- define "inferenceProxy.proxy.fullname" -}}
{{- printf "%s-proxy" (include "inferenceProxy.fullname" .) }}
{{- end }}

{{/*
Create environment variables
*/}}
{{- define "inferenceProxy.proxy.env" -}}
{{- $env := list }}
{{- if .Values.proxy.masterKey }}
{{- $env = append $env (dict "name" "INFERENCE_PROXY_MASTER_KEY" "valueFrom" (dict "secretKeyRef" (dict "name" "inference-secrets" "key" "INFERENCE_PROXY_MASTER_KEY"))) }}
{{- end }}
{{- $env = append $env (dict "name" "INFERENCE_PROXY_CONFIG" "value" "/etc/inference-proxy/config.yaml") }}
{{- if .Values.proxy.env }}
{{- range $k, $v := .Values.proxy.env }}
{{- $env = append $env (dict "name" $k "value" $v) }}
{{- end }}
{{- end }}
{{- return $env }}
{{- end }}
