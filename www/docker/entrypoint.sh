#!/bin/sh

set -e

if ! [ -d "node_modules" ]; then
  npm ci --dev
fi

exec "$@"

