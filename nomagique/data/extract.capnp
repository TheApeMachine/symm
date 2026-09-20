using Go = import "/go.capnp";
@0xf935a1768bf2027d;
$Go.package("data");
$Go.import("nomagique/data");

interface Extract {
  write @0 (
    in :Data,
    path :Text
  ) -> stream;
  done @1 () -> (out :Float64);
}
