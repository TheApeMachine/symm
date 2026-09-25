using Go = import "/go.capnp";
@0x9463cc5de0210879;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Chunk partitions a JSON array into deterministic sequential slices of up to
# size elements and issues an endpoint per populated shard.
interface Chunk {
  write @0 (data :Data, size :Int64, endpoint :Text) -> stream;
  done @1 () -> (
    out0 :Data,
    out1 :Data,
    out2 :Data,
    out3 :Data,
    endpoint0 :Text,
    endpoint1 :Text,
    endpoint2 :Text,
    endpoint3 :Text
  );
}
