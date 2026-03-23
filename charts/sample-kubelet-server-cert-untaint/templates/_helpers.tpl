{{/*
Expand the name of the chart.
*/}}
{{- define "kubelet-server-cert-untaint.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kubelet-server-cert-untaint.fullname" -}}
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
Common labels
*/}}
{{- define "kubelet-server-cert-untaint.labels" -}}
app.kubernetes.io/name: {{ include "kubelet-server-cert-untaint.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
k8s-app: {{ include "kubelet-server-cert-untaint.name" . }}
{{- if .Values.customLabels }}
{{ toYaml .Values.customLabels }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubelet-server-cert-untaint.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubelet-server-cert-untaint.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
k8s-app: {{ include "kubelet-server-cert-untaint.name" . }}
{{- end }}
