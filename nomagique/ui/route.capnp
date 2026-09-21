@0xd3478951e735bd48;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

using import "section.capnp".Section;

struct Route {
	path     @0 :Text;
	title    @1 :Text;
	sections @2 :List(Section);
}

interface UIRoute {
	write @0 (
		path :Text,
		title :Text,
		sections :List(Section)
	) -> stream;
	done @1 ();
}
