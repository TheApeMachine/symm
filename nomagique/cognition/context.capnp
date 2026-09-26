@0xf7ca931581c42d06;
using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");
using import "precursor.capnp".Context;
# Constructs the two legal inventory contexts for one causal observation.
# An inactive token set emits idle. Replay requires a nonempty explicit history.
interface ContextBuilder {
 write @0 (symbol :Text, vocabulary :Text, tokens :List(Text), history :List(Text), replay :Bool) -> stream;
 done @1 () -> Contexts;
}
struct Contexts {
 union {
  idle @0 :Void;
  ready :group {
   flat @1 :Context;
   held @2 :Context;
  }
 }
}
