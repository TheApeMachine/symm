using Go = import "/go.capnp";
@0xd0fb0fb33ce02b1c;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

# Nonce emits a strictly increasing decimal number, as ASCII digits, each time
# it is triggered. It starts from the clock so a restarted process keeps
# increasing past the nonces its previous run used.
interface Nonce {
  write @0 (trigger :Data) -> stream;
  done @1 () -> (out :Data);
}
