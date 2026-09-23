using Go = import "/go.capnp";
@0xb9881bfbc9aad278;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Queued;

# Capture preserves the exact frame and the ingress identity supplied by its source.
# Symbol/kind are optional metadata; absence is not replaced with an invented value.
#
# Every port gathers, so frames from every socket wired into one capture share
# its session and its sequence: slot n of payload, endpoint, receivedAt, symbol
# and kind describe one frame. Frames arriving in the same evaluation take
# consecutive sequence numbers in slot order and are handed out one row per
# evaluation; pending counts the rows still to hand out.
interface Capture extends(Queued) {
  write @0 (payload :List(Data), endpoint :List(Text), receivedAt :List(Text), symbol :List(Text), kind :List(Text)) -> stream;
  done @1 () -> Captured;
}

struct Captured {
  union {
    idle @0 :Void;
    row :group {
      out      @1 :Data;
      payload  @2 :Data;
      session  @3 :Text;
      sequence @4 :Int64;
      endpoint @5 :Text;
    }
  }
  pending @6 :UInt64;
}
