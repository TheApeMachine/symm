using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("nomagique/store");

interface Grid {
  write @0 (in :Data, metrics :Text) -> stream;
  done @1 () -> (out :Data);
}
