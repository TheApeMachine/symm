@0xd4e5b2ed5b75bf08;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

struct Component {
	name       @0 :Text;
	className  @1 :Text;
	propsJson  @2 :Text;
	components @3 :List(Component);
}

interface UIComponent {
	write @0 (
		name       :Text,
		className  :Text,
		propsJson  :Text,
		components :List(Component)
	) -> stream;
	done @1 () -> (out :Component);
}
