{{/*
Expand the name of the chart.
*/}}
{{- define "smtp-relay.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "smtp-relay.fullname" -}}
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
{{- define "smtp-relay.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "smtp-relay.labels" -}}
helm.sh/chart: {{ include "smtp-relay.chart" . }}
{{ include "smtp-relay.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: email-processor
app.kubernetes.io/part-of: mednet-email-infrastructure
{{- end }}

{{/*
Selector labels
*/}}
{{- define "smtp-relay.selectorLabels" -}}
app.kubernetes.io/name: {{ include "smtp-relay.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "smtp-relay.serviceAccountName" -}}
{{- if .Values.security.serviceAccount.create }}
{{- default (include "smtp-relay.fullname" .) .Values.security.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.security.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use
*/}}
{{- define "smtp-relay.secretName" -}}
{{- printf "%s-secret" (include "smtp-relay.fullname" .) }}
{{- end }}

{{/*
Create the name of the configmap to use
*/}}
{{- define "smtp-relay.configMapName" -}}
{{- printf "%s-config" (include "smtp-relay.fullname" .) }}
{{- end }}

{{/*
MySQL fullname helper
*/}}
{{- define "smtp-relay.mysql.fullname" -}}
{{- if .Values.mysql.enabled }}
{{- printf "%s-mysql" (include "smtp-relay.fullname" .) }}
{{- else }}
{{- .Values.externalMySQL.host }}
{{- end }}
{{- end }}

{{/*
Redis fullname helper
*/}}
{{- define "smtp-relay.redis.fullname" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis-master" (include "smtp-relay.fullname" .) }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Create MySQL connection URL
*/}}
{{- define "smtp-relay.mysql.url" -}}
{{- if .Values.mysql.enabled }}
{{- printf "mysql://%s:%s@%s:3306/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci" .Values.mysql.auth.username .Values.mysql.auth.password (include "smtp-relay.mysql.fullname" .) .Values.mysql.auth.database }}
{{- else }}
{{- .Values.externalMySQL.url }}
{{- end }}
{{- end }}

{{/*
Generate passwords for MySQL and Redis if not provided
*/}}
{{- define "smtp-relay.mysql.password" -}}
{{- if .Values.mysql.auth.password }}
{{- .Values.mysql.auth.password }}
{{- else }}
{{- randAlphaNum 16 }}
{{- end }}
{{- end }}

{{- define "smtp-relay.mysql.rootPassword" -}}
{{- if .Values.mysql.auth.rootPassword }}
{{- .Values.mysql.auth.rootPassword }}
{{- else }}
{{- randAlphaNum 20 }}
{{- end }}
{{- end }}

{{- define "smtp-relay.redis.password" -}}
{{- if .Values.redis.auth.password }}
{{- .Values.redis.auth.password }}
{{- else }}
{{- randAlphaNum 16 }}
{{- end }}
{{- end }}

{{/*
Validate configuration
*/}}
{{- define "smtp-relay.validateConfig" -}}
{{- if and (not .Values.mysql.enabled) (not .Values.externalMySQL.host) }}
{{- fail "Either mysql.enabled must be true or externalMySQL.host must be provided" }}
{{- end }}
{{- if and .Values.externalServices.gmail.workspaces .Values.externalServices.mailgun.workspaces }}
{{- if and (eq (len .Values.externalServices.gmail.workspaces) 0) (eq (len .Values.externalServices.mailgun.workspaces) 0) }}
{{- fail "At least one workspace must be configured in either gmail or mailgun" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Resource limits validation
*/}}
{{- define "smtp-relay.validateResources" -}}
{{- $memoryLimit := .Values.smtpRelay.resources.limits.memory | toString }}
{{- $memoryRequest := .Values.smtpRelay.resources.requests.memory | toString }}
{{- if lt ($memoryLimit | regexFind "[0-9]+" | int) ($memoryRequest | regexFind "[0-9]+" | int) }}
{{- fail "Memory limit must be greater than or equal to memory request" }}
{{- end }}
{{- end }}

{{/*
Medical compliance labels
*/}}
{{- define "smtp-relay.complianceLabels" -}}
{{- if .Values.compliance.medical.enabled }}
compliance.mednet.com/medical: "true"
compliance.mednet.com/data-retention: {{ .Values.compliance.medical.dataRetention | quote }}
{{- end }}
{{- if .Values.compliance.hipaa.enabled }}
compliance.mednet.com/hipaa: "true"
{{- end }}
compliance.mednet.com/data-sovereignty: {{ .Values.compliance.dataSovereignty.region | quote }}
{{- end }}

{{/*
Security annotations
*/}}
{{- define "smtp-relay.securityAnnotations" -}}
{{- if .Values.compliance.medical.encryption.atRest }}
encryption.mednet.com/at-rest: "true"
{{- end }}
{{- if .Values.compliance.medical.encryption.inTransit }}
encryption.mednet.com/in-transit: "true"
{{- end }}
{{- if .Values.compliance.medical.audit.enabled }}
audit.mednet.com/enabled: "true"
audit.mednet.com/retention: {{ .Values.compliance.medical.audit.retention | quote }}
{{- end }}
{{- end }}

{{/*
Performance annotations
*/}}
{{- define "smtp-relay.performanceAnnotations" -}}
performance.mednet.com/connection-pool-size: {{ .Values.performance.database.maxConnections | quote }}
performance.mednet.com/cache-size: {{ .Values.performance.cache.size | quote }}
performance.mednet.com/cache-ttl: {{ .Values.performance.cache.ttl | quote }}
{{- end }}

{{/*
Monitoring labels
*/}}
{{- define "smtp-relay.monitoringLabels" -}}
{{- if .Values.monitoring.prometheus.enabled }}
monitoring.mednet.com/prometheus: "true"
{{- end }}
{{- if .Values.monitoring.grafana.enabled }}
monitoring.mednet.com/grafana: "true"
{{- end }}
{{- if .Values.monitoring.alerting.enabled }}
monitoring.mednet.com/alerting: "true"
{{- end }}
{{- end }}

{{/*
Environment-specific configuration
*/}}
{{- define "smtp-relay.environment" -}}
{{- if contains "prod" .Release.Namespace }}
environment: production
{{- else if contains "stage" .Release.Namespace }}
environment: staging
{{- else if contains "dev" .Release.Namespace }}
environment: development
{{- else }}
environment: unknown
{{- end }}
{{- end }}

{{/*
Backup configuration validation
*/}}
{{- define "smtp-relay.validateBackup" -}}
{{- if .Values.backup.database.enabled }}
{{- if not .Values.backup.database.s3.bucket }}
{{- fail "S3 bucket must be specified when database backup is enabled" }}
{{- end }}
{{- if not .Values.backup.database.s3.region }}
{{- fail "S3 region must be specified when database backup is enabled" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Load balancer annotations for AWS
*/}}
{{- define "smtp-relay.awsLoadBalancerAnnotations" -}}
service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
service.beta.kubernetes.io/aws-load-balancer-scheme: "internal"
service.beta.kubernetes.io/aws-load-balancer-backend-protocol: "tcp"
service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"
{{- if .Values.compliance.medical.encryption.inTransit }}
service.beta.kubernetes.io/aws-load-balancer-ssl-cert: "arn:aws:acm:{{ .Values.compliance.dataSovereignty.region }}:ACCOUNT:certificate/CERTIFICATE"
service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "smtp,https"
{{- end }}
{{- end }}