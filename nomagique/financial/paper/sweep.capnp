@0xc8ff75b27bdab2e7;

using Go = import "/go.capnp";
$Go.package("paper");
$Go.import("github.com/theapemachine/symm/nomagique/financial/paper");

# Sweep walks price levels, best first, the way a market order takes them.
#
# With spend, amount is a quote budget: whole levels are bought while it pays
# for them, and the level it cannot pay for in full gives the largest multiple
# of increment it can. Without spend, amount is a base quantity delivered into
# the levels. What the levels could not absorb is unfilled, never assumed
# filled beyond what they show.
interface Sweep {
  write @0 () -> stream;
  done @1 () -> (ready :Bool);
  # One atomic request owns its result even when accounts share this capability.
  execute @2 (levels :List(import "book.capnp".Market.Level), amount :Text,
              increment :Text, spend :Bool) -> (quantity :Text, cost :Text, unfilled :Text);
}
