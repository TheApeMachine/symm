using Go = import "/go.capnp";
@0xe0d65bbf11e2f796;
$Go.package("ui");
$Go.import("nomagique/ui");

interface Broadcast {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
