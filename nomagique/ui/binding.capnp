@0xe3b1c2d4a5f60718;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

# Bound is one value that arrived at a component's input port. The component
# is named where it was authored — its graph and its id there — because that is
# the graph a surface draws, whatever definition nested it into the running
# program. The value is the JSON the port carries.
struct Bound {
	graph     @0 :Text;
	component @1 :Text;
	prop      @2 :Text;
	value     @3 :Text;
}

# Bindings are the values that reached component ports in one evaluation.
struct Bindings {
	values @0 :List(Bound);
}
