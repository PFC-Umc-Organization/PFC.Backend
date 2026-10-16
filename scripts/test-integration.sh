#!/usr/bin/env bash
# Testes de integração: precisa do DynamoDB Local em DYNAMODB_LOCAL (padrão http://localhost:8000).
set -euo pipefail
cd "$(dirname "$0")/.."

export AWS_ENDPOINT_URL_DYNAMODB="${DYNAMODB_LOCAL:-http://localhost:8000}"
export AWS_ENDPOINT_URL_COGNITO_IDENTITY_PROVIDER="http://127.0.0.1:18766"
export REFERENCIAS_CROSSREF_URL="http://127.0.0.1:18767"
export REFERENCIAS_OPENALEX_URL="http://127.0.0.1:18768"
export AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local
export TABLE_NAME=pfc-integracao GSI_NAME=GSI1
export COGNITO_CLIENT_ID=cliente-local COGNITO_USER_POOL_ID=us-east-1_local

go test ./internal/integracao -count=1 "$@"
