#!/bin/sh
set -e

# Run reflection in check mode
node --experimental-strip-types scripts/reflect-components.ts --check

