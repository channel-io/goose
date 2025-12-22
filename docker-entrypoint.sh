#!/bin/bash
set -e

GOOSE_MIGRATION_DIR="${GOOSE_MIGRATION_DIR:-/migrations}"
GOOSE_COMMAND="${GOOSE_COMMAND:-status}"

if [ ! -d "$GOOSE_MIGRATION_DIR" ]; then
    echo "Error: Migration directory not found: $GOOSE_MIGRATION_DIR"
    echo "Mount your migrations with: -v ./migrations:/migrations"
    exit 1
fi

if [ -z "$GOOSE_DRIVER" ]; then
    echo "Error: GOOSE_DRIVER is required"
    echo "Example: -e GOOSE_DRIVER=starrocks"
    exit 1
fi

if [ -z "$GOOSE_DBSTRING" ]; then
    echo "Error: GOOSE_DBSTRING is required"
    echo "Example: -e GOOSE_DBSTRING='root:@tcp(localhost:9030)/mydb'"
    exit 1
fi

export GOOSE_MIGRATION_DIR

if [ $# -eq 0 ]; then
    exec goose -dir "$GOOSE_MIGRATION_DIR" "$GOOSE_COMMAND"
else
    exec goose -dir "$GOOSE_MIGRATION_DIR" "$@"
fi
