@0xd3478951e735bd48;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

using import "component.capnp".Component;

struct Route {
	path       @0 :Text;
	title      @1 :Text;
	components @2 :List(Component);
}

interface UIRoute {
	write @0 (
		path       :Text,
		title      :Text,
		components :List(Component)
	) -> stream;
	done @1 () -> (out :Route);
}
