package command

import "fmt"

// Registry contains only runnable operations. Future command ideas stay out of
// discovery until the provider implementation exists, so humans and agents can
// trust this catalog as an executable contract.
var Registry = []Definition{
	{Effect: Observe, Path: []string{"trello", "board", "list"}, Summary: "List boards visible to the configured Trello user.", Description: "Discover Trello boards available to the configured user before selecting a board ID.", Response: "A collection of board IDs and names.", Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "board", "overview"}, Summary: "Read a board and its lists with card counts.", Description: "Return a selected board and its lists. Uses trello.default-board-id when BOARD_ID is omitted.", Response: "A board record and collection of list records.", Arguments: []Argument{{Name: "BOARD_ID", Description: "Trello board ID. Optional when trello.default-board-id is configured.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "list", "list"}, Summary: "List the lists on one board.", Description: "Return list records for BOARD_ID or the configured default board.", Response: "A collection of Trello list records.", Arguments: []Argument{{Name: "BOARD_ID", Description: "Trello board ID. Optional when trello.default-board-id is configured.", Optional: true}}, Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "card", "get"}, Summary: "Read one Trello card.", Description: "Return the Trello API record for one card without changing it.", Response: "One Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Create, Path: []string{"trello", "card", "create"}, Summary: "Create one Trello card.", Description: "Create a card in LIST_ID with a required name and optional description or due date.", Response: "The created Trello card record.", Arguments: []Argument{{Name: "LIST_ID", Description: "Trello list ID that receives the card.", Optional: false}}, Options: []Option{{Name: "--name", Value: "NAME", Description: "Card title.", Optional: false}, {Name: "--description", Value: "TEXT", Description: "Card description.", Optional: true}, {Name: "--due", Value: "DATE", Description: "Card due date accepted by Trello.", Optional: true}}, Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false},
	{Effect: Move, Path: []string{"trello", "card", "move"}, Summary: "Move one Trello card to a list or position.", Description: "Move CARD_ID to LIST_ID. This changes workflow placement but keeps the card.", Response: "The updated Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}, {Name: "LIST_ID", Description: "Destination Trello list ID.", Optional: false}}, Options: []Option{{Name: "--position", Value: "POSITION", Description: "Destination position; defaults to bottom.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: false},
	{Effect: Archive, Path: []string{"trello", "card", "archive"}, Summary: "Archive one Trello card.", Description: "Close one card while preserving it in Trello's archive.", Response: "The archived Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: false},
	{Effect: Delete, Path: []string{"trello", "card", "delete"}, Summary: "Permanently delete one Trello card.", Description: "Permanently remove one Trello card.", Response: "Deletion confirmation containing the card ID.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}}, Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false},
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
