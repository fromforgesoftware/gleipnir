{{- define "conduit.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "conduit.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "conduit.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "conduit.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "conduit.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: forge
{{- end -}}

{{- define "conduit.selectorLabels" -}}
app.kubernetes.io/name: {{ include "conduit.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "conduit.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "conduit.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "conduit.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "conduit.dbSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else -}}
{{- printf "%s-db" (include "conduit.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "conduit.vaultSecretName" -}}
{{- if .Values.vault.existingSecret -}}
{{- .Values.vault.existingSecret -}}
{{- else -}}
{{- printf "%s-vault" (include "conduit.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Shared env block for server + migrator. */}}
{{- define "conduit.env" -}}
- name: SVC_NAME
  value: {{ include "conduit.name" . | quote }}
- name: REST_ADDRESS
  value: ":{{ .Values.ports.http }}"
- name: HTTP_ADDRESS
  value: ":{{ .Values.ports.http }}"
- name: GRPC_ADDRESS
  value: ":{{ .Values.ports.grpc }}"
- name: DB_HOST
  value: {{ .Values.database.host | quote }}
- name: DB_PORT
  value: {{ .Values.database.port | quote }}
- name: DB_NAME
  value: {{ .Values.database.name | quote }}
- name: DB_SCHEMA
  value: {{ .Values.database.schema | quote }}
- name: DB_SSL
  value: {{ .Values.database.ssl | quote }}
- name: DB_LOG_LEVEL
  value: {{ .Values.database.logLevel | default "warn" | quote }}
- name: DB_USER
  valueFrom:
    secretKeyRef:
      name: {{ include "conduit.dbSecretName" . }}
      key: DB_USER
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "conduit.dbSecretName" . }}
      key: DB_PASSWORD
{{- if .Values.vault.kmsKey }}
- name: CONDUIT_KMS_KEY
  value: {{ .Values.vault.kmsKey | quote }}
{{- else }}
- name: CONDUIT_KEY_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "conduit.vaultSecretName" . }}
      key: CONDUIT_KEY_ENCRYPTION_KEY
{{- end }}
{{- if .Values.gatewaySecret }}
- name: FORGE_GATEWAY_SECRET
  value: {{ .Values.gatewaySecret | quote }}
{{- end }}
{{- end -}}
