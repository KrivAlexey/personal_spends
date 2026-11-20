#!/usr/bin/env bash
# Wipes the compose stack (DynamoDB Local, Postgres, S3Mock: all in memory) and
# recreates it empty, with the DynamoDB table applied from the `local` Terraform
# workspace. Never touches real AWS: in that workspace the endpoint is localhost:8000.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
unset TF_WORKSPACE  # an exported one would redirect the workspace commands below
# The server creates its Postgres table only at startup, so one kept across the reset breaks.
curl -s --max-time 2 -o /dev/null http://localhost:8080/ && { echo "something listens on localhost:8080; stop the server first" >&2; exit 1; }

docker compose down
docker compose up -d
for i in $(seq 30); do
  # Any HTTP answer means DynamoDB Local is up; curl fails only on no connection.
  curl -s --max-time 2 -o /dev/null http://localhost:8000/ && break
  [[ $i -eq 30 ]] && { echo "DynamoDB Local did not answer on localhost:8000 within 30s" >&2; exit 1; }
  sleep 1
done

cd terraform
terraform init -input=false >/dev/null
# For the local backend a workspace is its folder; creating it this way leaves the saved selection alone.
mkdir -p terraform.tfstate.d/local
# The old state lists a table the restarted DynamoDB Local no longer has.
rm -f terraform.tfstate.d/local/terraform.tfstate terraform.tfstate.d/local/terraform.tfstate.backup
# TF_WORKSPACE scopes the workspace to this one command.
# -auto-approve is safe only because `local` is pinned to the emulator in main.tf.
TF_WORKSPACE=local terraform apply -auto-approve -input=false
