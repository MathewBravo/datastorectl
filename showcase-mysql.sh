#!/usr/bin/env bash
set -euo pipefail

BOOTSTRAP_PW="showcase_pw_4rEaDy"

case "${1:-help}" in
  up)
    echo "Starting MySQL..."
    docker compose -f docker-compose.mysql.yml up -d
    echo "Waiting for cluster to be healthy..."
    until docker exec datastorectl-mysql-1 mysqladmin ping -h localhost -uroot -pdatastorectl --silent 2>/dev/null; do
      sleep 2
    done
    echo "Cluster is ready."
    echo ""
    echo "Provisioning the datastorectl management account..."
    sed "s/<REPLACE_ME>/${BOOTSTRAP_PW}/" providers/mysql/bootstrap/bootstrap-readwrite.sql \
      | docker exec -i datastorectl-mysql-1 mysql -uroot -pdatastorectl 2>/dev/null
    echo "Bootstrap applied."
    echo ""
    echo "Building datastorectl..."
    go build -o datastorectl ./cmd/datastorectl
    echo "Done."
    echo ""
    echo "Run the showcase (export the passwords for secret() resolution):"
    echo ""
    echo "  export DATASTORECTL_MYSQL_PASSWORD=${BOOTSTRAP_PW}"
    echo "  export DATASTORECTL_MYSQL_APP_PW=app_demo_pw"
    echo "  export DATASTORECTL_MYSQL_OPS_PW=ops_demo_pw"
    echo ""
    echo "  ./datastorectl validate testdata/showcase-mysql/resources.dcl"
    echo "  ./datastorectl plan     testdata/showcase-mysql/resources.dcl"
    echo "  ./datastorectl apply    testdata/showcase-mysql/resources.dcl"
    echo "  ./datastorectl plan     testdata/showcase-mysql/resources.dcl   # converges"
    echo ""
    echo "To see the self-lockout guard fire, try:"
    echo "  ./datastorectl plan --prune testdata/showcase-mysql/resources.dcl"
    echo ""
    ;;
  down)
    echo "Stopping MySQL..."
    docker compose -f docker-compose.mysql.yml down
    echo "Done."
    ;;
  *)
    echo "Usage: ./showcase-mysql.sh [up|down]"
    echo ""
    echo "  up    Start MySQL + bootstrap datastorectl account + build binary"
    echo "  down  Stop MySQL"
    ;;
esac
