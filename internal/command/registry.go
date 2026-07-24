package command

import "fmt"

// Registry deliberately contains individual leaf operations. It does not offer
// a raw HTTP escape hatch: such a command would be Unbounded and cannot safely
// receive automatic approval.
var Registry = []Definition{
	{Effect: Observe, Path: []string{"tailscale", "device", "list"}, Summary: "List devices in the Tailnet.", Description: "Return devices visible to the configured Tailnet administrator.", Response: "A collection of Tailnet device records.", Cardinality: Many, Reversible: true, Credentials: "tailscale", UnattendedOK: true, Status: "planned"},
	{Effect: Observe, Path: []string{"tailscale", "device", "get"}, Summary: "Read one Tailnet device.", Description: "Return details for one device without changing Tailnet state.", Response: "One Tailnet device record.", Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true, Status: "planned"},
	{Effect: Observe, Path: []string{"tailscale", "acl", "validate"}, Summary: "Validate a policy file without applying it.", Description: "Validate Tailnet policy syntax and semantics without modifying the Tailnet.", Response: "Validation result and diagnostics.", Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true, Status: "planned"},
	{Effect: Authorize, Path: []string{"tailscale", "device", "authorize"}, Summary: "Authorize one Tailnet device.", Description: "Change authorization state for one device.", Response: "The updated device authorization record.", Cardinality: One, Reversible: false, Credentials: "tailscale", UnattendedOK: false, Status: "planned"},
	{Effect: Administer, Path: []string{"tailscale", "acl", "apply"}, Summary: "Apply Tailnet policy configuration.", Description: "Apply a reviewed Tailnet policy configuration.", Response: "Policy application result.", Cardinality: One, Reversible: false, Credentials: "tailscale", UnattendedOK: false, Status: "planned"},
	{Effect: Delete, Path: []string{"tailscale", "device", "delete"}, Summary: "Delete one device from the Tailnet.", Description: "Remove one device from the Tailnet.", Response: "Deletion confirmation.", Cardinality: One, Reversible: false, Credentials: "tailscale", UnattendedOK: false, Status: "planned"},

	{Effect: Observe, Path: []string{"trello", "board", "list"}, Summary: "List boards visible to the configured Trello user.", Description: "Discover Trello boards available to the configured user before selecting a board ID.", Response: "A collection of board IDs and names.", Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true, Status: "implemented"},
	{Effect: Observe, Path: []string{"trello", "board", "overview"}, Summary: "Read a board and its lists with card counts.", Description: "Return a selected board and its lists. Uses trello.default-board-id when BOARD_ID is omitted.", Response: "A board record and collection of list records.", Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true, Status: "implemented"},
	{Effect: Observe, Path: []string{"trello", "list", "list"}, Summary: "List the lists on one board.", Description: "Return list records for BOARD_ID or the configured default board.", Response: "A collection of Trello list records.", Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true, Status: "implemented"},
	{Effect: Observe, Path: []string{"trello", "card", "get"}, Summary: "Read one Trello card.", Description: "Return the Trello API record for one card without changing it.", Response: "One Trello card record.", Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true, Status: "implemented"},
	{Effect: Create, Path: []string{"trello", "board", "create"}, Summary: "Create one Trello board.", Description: "Create one board in Trello.", Response: "The created Trello board record.", Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false, Status: "planned"},
	{Effect: Create, Path: []string{"trello", "card", "create"}, Summary: "Create one Trello card.", Description: "Create a card in LIST_ID with a required name and optional description or due date.", Response: "The created Trello card record.", Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false, Status: "implemented"},
	{Effect: Move, Path: []string{"trello", "card", "move"}, Summary: "Move one Trello card to a list or position.", Description: "Move CARD_ID to LIST_ID with an optional position.", Response: "The updated Trello card record.", Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: false, Status: "implemented"},
	{Effect: Archive, Path: []string{"trello", "card", "archive"}, Summary: "Archive one Trello card.", Description: "Close one card while preserving it in Trello's archive.", Response: "The archived Trello card record.", Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: false, Status: "implemented"},
	{Effect: Delete, Path: []string{"trello", "card", "delete"}, Summary: "Permanently delete one Trello card.", Description: "Permanently remove one Trello card.", Response: "Deletion confirmation containing the card ID.", Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false, Status: "implemented"},
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
