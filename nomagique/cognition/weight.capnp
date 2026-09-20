@0x819d63e66671d350;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Weight {
  write @0 (record :Data) -> stream;
  done @1 () -> (
    count :UInt64,
    mass :UInt64,
    writeStep :UInt64
  );
}
