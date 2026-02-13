{{/*
Expand the name of the chart.
*/}}
{{- define "hortator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "hortator.fullname" -}}
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
{{- define "hortator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "hortator.labels" -}}
helm.sh/chart: {{ include "hortator.chart" . }}
{{ include "hortator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "hortator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hortator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "hortator.serviceAccountName" -}}
{{- if .Values.operator.serviceAccount.create }}
{{- default (include "hortator.fullname" .) .Values.operator.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.operator.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the operator image reference
*/}}
{{- define "hortator.operatorImage" -}}
{{- $tag := default (printf "v%s" .Chart.AppVersion) .Values.operator.image.tag }}
{{- printf "%s:%s" .Values.operator.image.repository $tag }}
{{- end }}

{{/*
Create the agent image reference
*/}}
{{- define "hortator.agentImage" -}}
{{- $tag := default (printf "v%s" .Chart.AppVersion) .Values.agent.image.tag }}
{{- printf "%s:%s" .Values.agent.image.repository $tag }}
{{- end }}

{{/*
Create the agentic agent image reference
*/}}
{{- define "hortator.agenticImage" -}}
{{- $tag := default (printf "v%s" .Chart.AppVersion) .Values.agent.agenticImage.tag }}
{{- printf "%s:%s" .Values.agent.agenticImage.repository $tag }}
{{- end }}
