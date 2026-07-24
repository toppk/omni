package command

import "fmt"

// Registry deliberately contains individual leaf operations. It does not offer
// a raw HTTP escape hatch: such a command would be Unbounded and cannot safely
// receive automatic approval.
var Registry = []Definition{
	{Observe, []string{"tailscale", "device", "list"}, "List devices in the Tailnet.", Many, true, "tailscale", true},
	{Observe, []string{"tailscale", "device", "get"}, "Read one Tailnet device.", One, true, "tailscale", true},
	{Observe, []string{"tailscale", "acl", "validate"}, "Validate a policy file without applying it.", One, true, "tailscale", true},
	{Authorize, []string{"tailscale", "device", "authorize"}, "Authorize one Tailnet device.", One, false, "tailscale", false},
	{Administer, []string{"tailscale", "acl", "apply"}, "Apply Tailnet policy configuration.", One, false, "tailscale", false},
	{Delete, []string{"tailscale", "device", "delete"}, "Delete one device from the Tailnet.", One, false, "tailscale", false},

	{Observe, []string{"trello", "board", "list"}, "List boards visible to the configured Trello user.", Many, true, "trello", true},
	{Observe, []string{"trello", "board", "overview"}, "Read a board and its lists with card counts.", One, true, "trello", true},
	{Observe, []string{"trello", "list", "list"}, "List the lists on one board.", Many, true, "trello", true},
	{Observe, []string{"trello", "card", "get"}, "Read one Trello card.", One, true, "trello", true},
	{Create, []string{"trello", "board", "create"}, "Create one Trello board.", One, false, "trello", false},
	{Create, []string{"trello", "card", "create"}, "Create one Trello card.", One, false, "trello", false},
	{Move, []string{"trello", "card", "move"}, "Move one Trello card to a list or position.", One, true, "trello", false},
	{Archive, []string{"trello", "card", "archive"}, "Archive one Trello card.", One, true, "trello", false},
	{Delete, []string{"trello", "card", "delete"}, "Permanently delete one Trello card.", One, false, "trello", false},
}

func Find(tokens []string) (Definition, error) {
	for _, d := range Registry {
		if same(d.Tokens(), tokens) {
			return d, nil
		}
	}
	return Definition{}, fmt.Errorf("unknown command %q", join(tokens))
}

// Match finds a complete command path at the left of tokens and returns the
// remaining resource identifiers and values. Those remaining arguments cannot
// alter the effect because the definition has already been selected.
func Match(tokens []string) (Definition, []string, error) {
	for _, d := range Registry {
		path := d.Tokens()
		if len(tokens) < len(path) || !same(path, tokens[:len(path)]) {
			continue
		}
		return d, tokens[len(path):], nil
	}
	return Definition{}, nil, fmt.Errorf("unknown command %q", join(tokens))
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func join(s []string) string {
	if len(s) == 0 {
		return ""
	}
	r := s[0]
	for _, v := range s[1:] {
		r += " " + v
	}
	return r
}

func init() {
	for _, d := range Registry {
		if err := d.Validate(); err != nil {
			panic(err)
		}
	}
}
