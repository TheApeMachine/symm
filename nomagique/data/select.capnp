using Go = import "/go.capnp";
@0x9ffa62d694ec8b4f;
$Go.package("data");
$Go.import("nomagique/data");

interface Select {
  write @0 (data :Data, path :Text) -> stream;
  done @1 () -> (out :Float64);
}
