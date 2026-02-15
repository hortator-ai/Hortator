{{/*
LiteLLM fullname - uses parent release name.
*/}}
{{- define "litellm.fullname" -}}
{{- printf "%s-litellm" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
LiteLLM common labels.
*/}}
{{- define "litellm.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "litellm.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
LiteLLM selector labels.
*/}}
{{- define "litellm.selectorLabels" -}}
app.kubernetes.io/name: litellm
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: litellm-proxy
{{- end }}

{{/*
LiteLLM secret name for API keys.
*/}}
{{- define "litellm.secretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- include "litellm.fullname" . }}
{{- end }}
{{- end }}

{{/*
LiteLLM master key secret name.
*/}}
{{- define "litellm.masterKeySecretName" -}}
{{- if .Values.existingMasterKeySecret }}
{{- .Values.existingMasterKeySecret }}
{{- else }}
{{- include "litellm.fullname" . }}
{{- end }}
{{- end }}
