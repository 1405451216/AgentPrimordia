{{/*
AgentPrimordia Helm Chart 辅助模板
*/}}

{{/*
Chart 全名
*/}}
{{- define "agentprimordia.fullname" -}}
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
Chart 名称
*/}}
{{- define "agentprimordia.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
通用标签
*/}}
{{- define "agentprimordia.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "agentprimordia.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
选择器标签
*/}}
{{- define "agentprimordia.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentprimordia.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount 名称
*/}}
{{- define "agentprimordia.serviceAccountName" -}}
{{- printf "%s-sa" (include "agentprimordia.fullname" .) }}
{{- end }}

{{/*
etcd 端点
*/}}
{{- define "agentprimordia.etcdEndpoints" -}}
{{- if .Values.etcd.endpoints }}
{{- .Values.etcd.endpoints }}
{{- else }}
{{- printf "http://%s-etcd:2379" (include "agentprimordia.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Studio 全名
*/}}
{{- define "agentprimordia.studioFullname" -}}
{{- printf "%s-studio" (include "agentprimordia.fullname" .) }}
{{- end }}
