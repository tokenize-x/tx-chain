package deterministicmap

import "fmt"

func SomeFunc() {
	m := map[string]int{"a": 1}
	for k, v := range m /* want "ranging over map is forbidden (iteration order is nondeterministic); use DeterministicMap instead" */ {
		fmt.Println(k, v)
	}
}
