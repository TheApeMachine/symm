@0xe0717a3ad228d5ef;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

using import "../store/radix.capnp".Retained;

interface TokenSequence extends(Retained) {
  write @0 (
    scope  :Text,
    token  :Text,
    tokens :List(Text),
    reset  :Bool
  ) -> stream;
  done @1 () -> (
    sequence :List(Text),
    path     :Text,
    depth    :Int64,
    out      :Data
  );
}
