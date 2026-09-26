using Go = import "/go.capnp";
@0xb9881bfbc9aad278;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Queued;

# Capture preserves the exact frame and the ingress identity supplied by its source.
# Symbol/kind are optional metadata; absence is not replaced with an invented value.
#
# Source-owned provenance accompanies every payload. Capture retains those identities;
# it does not assign a second sequence. Pending rows drain in arrival slot order.
interface Capture extends(Queued) {
  write @0 (payload :List(Data), provenance :List(Data), symbol :List(Text), kind :List(Text)) -> stream;
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
