package sim

// Identity tags grant behavior, while the resolver computes its effects.
var tagActions = map[string][]string{
	"resident": {"wait", "move", "eat", "speak"},
	"farmer":   {"work"},
	"gatherer": {"work"},
	"trader":   {"offer", "accept", "reject"},
	"fighter":  {"attack"},
	"healer":   {"heal"},
}

func Capabilities(a Actor) map[string]bool {
	result := map[string]bool{}
	for _, tag := range a.Tags {
		for _, action := range tagActions[tag] {
			result[action] = true
		}
	}
	return result
}
