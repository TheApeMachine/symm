using Go = import "/go.capnp";
@0xca453210b7aa0125;
$Go.package("kraken");
$Go.import("github.com/theapemachine/symm/nomagique/financial/kraken");

# Futures reads a Kraken Futures websocket frame into the records the signal
# graph reads, on channels of their own so a futures quote or fill is never
# taken for a spot one:
#
#   futures_ticker  data {symbol, last, bid, ask, bid_qty, ask_qty, index,
#                         mark, openInterest, timestamp}
#   futures_trade   data {symbol, price, qty, side, type, timestamp}
#
# type is the venue's fill type (fill, liquidation, termination, block). A
# trade snapshot is every trade in it, oldest first. timestamp is the venue's
# millisecond time written as RFC 3339. The capture the frame arrived with, if
# any, is carried on every record. A frame that is not a ticker or trade feed
# (subscription replies, heartbeats) is idle; a frame that is not JSON is an
# error.
interface Futures {
  write @0 (data :Data) -> stream;
  done @1 () -> Records;
}

struct Records {
  union {
    idle    @0 :Void;
    records @1 :List(Data);
  }
}
