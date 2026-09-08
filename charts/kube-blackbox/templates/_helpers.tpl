{{- define "kube-blackbox.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kube-blackbox.fullname" -}}
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

{{- define "kube-blackbox.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kube-blackbox.labels" -}}
helm.sh/chart: {{ include "kube-blackbox.chart" . }}
{{ include "kube-blackbox.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{- define "kube-blackbox.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kube-blackbox.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "kube-blackbox.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kube-blackbox.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- required "serviceAccount.name is required when serviceAccount.create=false" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "kube-blackbox.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (required "image.tag is required when image.digest is empty" .Values.image.tag) }}
{{- end }}
{{- end }}

{{- define "kube-blackbox.pvcName" -}}
{{- default (printf "%s-data" (include "kube-blackbox.fullname" .)) .Values.persistence.existingClaim }}
{{- end }}

{{- define "kube-blackbox.clusterRoleName" -}}
{{- printf "%s-%s-readonly" (include "kube-blackbox.fullname" .) .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end }}
