@0xf8ef5912340eab62;
using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");
using import "../store/radix.capnp".Radix;

# Context carries causal selection inputs. Outcome labels are deliberately absent.
struct Context {
 symbol @0 :Text;
 holding @1 :Bool;
 vocabulary @2 :Text;
 tokens @3 :List(Text);
 union {
  live @4 :Void;
  history @5 :List(Text);
 }
}

# Reversed, length-delimited histories permit longest-suffix recognition.
# Only the radix key is encoded; the context itself is never serialized to JSON.
interface Precursor {
 write @0 (context :Context, model :Radix) -> stream;
 done @1 () -> Preceded;
}

struct Preceded {
 union {
 idle @0 :Void;
 ready :group { key @1 :Text; prefix @2 :Bool; }
 }
}
