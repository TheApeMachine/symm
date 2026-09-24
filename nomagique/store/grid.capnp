using Go = import "/go.capnp";
@0xa66318359218d6a8;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Status;
using import "radix.capnp".Retained;

# Grid is the virtual grid. Raw market data is written to it and the metrics
# wired into it receive the fields they declared an interest in.
#
# Every feed lands on the one data port. It gathers rather than replaces, so
# adding a venue is wiring one more producer into it rather than widening the
# schema.
#
# What a metric asked for comes back out already typed, one slot per declared
# interest and in the order they were declared, so a metric is handed the
# numbers it needs rather than a document it has to go looking through.
#
# A record carries the fields of the feed it came from and no others, so most
# deliveries fill only some of the slots. Every declared interest still gets
# its own slot, and present says which ones the record actually carried. A
# metric waiting on a slot that stayed empty keeps waiting; it is never handed
# a zero standing in for a field that was not there, and never reads its
# neighbour's field because an absent one closed the gap.
#
# The grid holds no values of its own. A metric is not a cell holding a number
# but a capability the grid calls, so reading the grid is asking every metric
# wired into it for its current state. Wiring another metric in is what makes
# the grid wider; nothing here enumerates them.
# The grid is Retained: what it hands out is what it was holding when the
# evaluation began. That is what lets the same grid feed the metrics and
# collect them, without the two closing a cycle around each other.
#
# scope names the series the written data belongs to; like data it gathers,
# and every scope written together must agree. A new scope starts the grid
# from no retained readings, so nothing observed under one series is handed
# out under the next, and scope is handed back out so every metric reading the
# grid reads under the same one. No scope arriving keeps the current series;
# an unwired scope is one series.
interface Grid extends(Retained) {
  write @0 (
    data      :List(Data),
    interests :Text,
    metrics   :List(Float64),
    present   :List(Bool),
    scope     :List(Text)
  ) -> stream;
  done @1 () -> (
    values       :List(Float64),
    present      :List(Bool),
    out          :Data,
    delivered    :Int64,
    metrics      :Int64,
    status       :Status,
    observations :List(Float64),
    observed     :List(Bool),
    scope        :Text
  );
}
