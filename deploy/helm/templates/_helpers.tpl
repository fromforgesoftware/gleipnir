{{- define "gleipnir.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gleipnir.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "gleipnir.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "gleipnir.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "gleipnir.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: forge
{{- end -}}

{{- define "gleipnir.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gleipnir.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "gleipnir.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "gleipnir.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "gleipnir.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{- define "gleipnir.dbSecretName" -}}
{{- if .Values.database.existingSecret -}}
{{- .Values.database.existingSecret -}}
{{- else -}}
{{- printf "%s-db" (include "gleipnir.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "gleipnir.vaultSecretName" -}}
{{- if .Values.vault.existingSecret -}}
{{- .Values.vault.existingSecret -}}
{{- else -}}
{{- printf "%s-vault" (include "gleipnir.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "gleipnir.oauthSecretName" -}}
{{- if .Values.oauth.existingSecret -}}
{{- .Values.oauth.existingSecret -}}
{{- else -}}
{{- printf "%s-oauth" (include "gleipnir.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* Shared env block for server + migrator. */}}
{{- define "gleipnir.env" -}}
- name: SVC_NAME
  value: {{ include "gleipnir.name" . | quote }}
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
      name: {{ include "gleipnir.dbSecretName" . }}
      key: DB_USER
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "gleipnir.dbSecretName" . }}
      key: DB_PASSWORD
{{- if .Values.vault.kmsKey }}
- name: GLEIPNIR_KMS_KEY
  value: {{ .Values.vault.kmsKey | quote }}
{{- else }}
- name: GLEIPNIR_KEY_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "gleipnir.vaultSecretName" . }}
      key: GLEIPNIR_KEY_ENCRYPTION_KEY
{{- end }}
{{- range .Values.oauth.connectors }}
- name: GLEIPNIR_OAUTH_{{ .slug | upper }}_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "gleipnir.oauthSecretName" $ }}
      key: GLEIPNIR_OAUTH_{{ .slug | upper }}_CLIENT_ID
- name: GLEIPNIR_OAUTH_{{ .slug | upper }}_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ include "gleipnir.oauthSecretName" $ }}
      key: GLEIPNIR_OAUTH_{{ .slug | upper }}_CLIENT_SECRET
{{- end }}
{{- if .Values.oauth.redirectUris }}
- name: GLEIPNIR_OAUTH_REDIRECT_URIS
  value: {{ join "," .Values.oauth.redirectUris | quote }}
{{- end }}
{{- if .Values.gatewaySecret }}
- name: FORGE_GATEWAY_SECRET
  value: {{ .Values.gatewaySecret | quote }}
{{- end }}
{{- end -}}
