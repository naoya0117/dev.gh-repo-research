#!/bin/sh
if ! [ -d "node_modules" ]; then
  npm ci --dev
fi

npm run start

exec "$@"