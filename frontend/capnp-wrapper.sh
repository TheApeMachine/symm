#!/bin/bash
# The generator shells out to `capnp compile` without an include path, but the
# schemas import /go.capnp from the Go capnp module. This wrapper supplies that
# include path, resolved from the module cache rather than hardcoded, so the
# generator runs wherever the module is.
set -euo pipefail

module=$(cd .. && go list -m -f '{{.Dir}}' capnproto.org/go/capnp/v3)

exec capnp "$@" -I "${module}/std"
