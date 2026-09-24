@0xb62908f5d0234a62;

using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Status;
using import "radix.capnp".Retained;

# Revision owns a keyed collection of evidence documents and hands out
# frozen immutable revisions on done.
interface Revision extends(Retained) {
  write @0 (
    key   :Text,
    data  :Data,
    flush :Bool,
    reset :Bool
  ) -> stream;
  done @1 () -> (
    status   :Status,
    revision :Int64,
    count    :Int64,
    out      :Data
  );
}
