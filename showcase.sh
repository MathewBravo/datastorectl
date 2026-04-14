#!/usr/bin/env bash
set -euo pipefail

case "${1:-help}" in
  up)
    echo "Starting OpenSearch..."
    docker compose up -d
    echo "Waiting for cluster to be healthy..."
    until curl -sk https://localhost:9200/_cluster/health \
      -u admin:myStrongPassword123! | grep -q '"status"'; do
      sleep 2
    done
    echo "Cluster is ready."
    echo ""
    echo "Building datastorectl..."
    go build -o datastorectl ./cmd/datastorectl
    echo "Done. Run the showcase:"
    echo ""
    echo "  ./datastorectl validate testdata/showcase/resources.dcl"
    echo "  ./datastorectl plan testdata/showcase/resources.dcl"
    echo "  ./datastorectl apply testdata/showcase/resources.dcl"
    echo "  ./datastorectl plan testdata/showcase/resources.dcl"
    echo ""
    ;;
  down)
    echo "Stopping OpenSearch..."
    docker compose down
    echo "Done."
    ;;
  *)
    echo "Usage: ./showcase.sh [up|down]"
    echo ""
    echo "  up    Start OpenSearch + build datastorectl"
    echo "  down  Stop OpenSearch"
    ;;
esac
