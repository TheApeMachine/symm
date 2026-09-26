@0xe0717a3ad228d5ef;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

using import "../store/radix.capnp".Retained;

# TokenSequence keeps one token history per scope: each step appends the
# tokens that arrived to its scope's history, and publishes the scope's
# history as a path whether or not this step added to it: the development so
# far is what the step is read against. A scope with no history yet is idle.
# reset starts a new window: every scope's history is cleared before the
# step, so nothing from before the window is carried into it.
interface TokenSequence extends(Retained) {
  write @0 (
    scope  :Text,
    token  :Text,
    tokens :List(Text),
    reset  :Bool
  ) -> stream;
  done @1 () -> Sequenced;
}

struct Sequenced {
  union {
    idle @0 :Void;
    step :group {
      sequence @1 :List(Text);
      path     @2 :Text;
      depth    @3 :Int64;
    }
  }
}
