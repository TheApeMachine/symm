using Go = import "/go.capnp";
@0xb9881bfbc9aad278;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Capture preserves the exact frame and the ingress identity supplied by its source.
# Symbol/kind are optional metadata; absence is not replaced with an invented value.
interface Capture {
  write @0 (payload :Data, endpoint :Text, receivedAt :Text, symbol :Text, kind :Text) -> stream;
  done @1 () -> (out :Data);
}
