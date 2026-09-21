using Go = import "/go.capnp";
@0xa17c3d5e92f4b806;
$Go.package("transport");
$Go.import("nomagique/transport");

using import "../runtime/status.capnp".Status;

# Disruptor publishes an observation into an LMAX ring and reports it to the
# stages mounted behind each barrier. A stage is wired by connecting its input
# to one of the stage ports: everything wired to stage1 runs concurrently as
# one handler group, and nothing on stage2 observes a sequence until every
# handler on stage1 has passed it.
interface Disruptor {
  write @0 (
    data     :Data,
    capacity :Int64,
    writers  :Int64,
    admit    :Bool
  ) -> stream;
  done @1 () -> (
    stage1  :Data,
    stage2  :Data,
    stage3  :Data,
    stage4  :Data,
    backlog :Int64,
    status  :Status
  );
}
