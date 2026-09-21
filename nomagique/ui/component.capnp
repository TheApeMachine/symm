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

# Children are graph structure, so a parent is wired to the components
# themselves rather than to copies of what they rendered. Wiring one more in is
# what makes a parent wider.
interface UIComponent {
	write @0 (
		name       :Text,
		className  :Text,
		propsJson  :Text,
		components :List(UIComponent)
	) -> stream;
	done @1 () -> (out :Component);
}
