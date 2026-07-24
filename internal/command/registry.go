package command

import "fmt"

// Registry contains only runnable operations. Future command ideas stay out of
// discovery until the provider implementation exists, so humans and agents can
// trust this catalog as an executable contract.
var Registry = []Definition{
	{Effect: Observe, Path: []string{"tailscale", "device", "list"}, Summary: "List devices in the configured tailnet.", Description: "Return only each device's ID, hostname, operating system, and last-seen time. Use --tag to audit a device identity such as an ephemeral CI runner, or --details for selection context without fetching every device individually.", Response: "A compact collection of device records.", Options: []Option{{Name: "--tag", Value: "TAG", Description: "Return only devices currently carrying TAG, for example tag:github.", Optional: true}, {Name: "--details", Description: "Include name, owner, addresses, tags, authorization, client version, and key expiry.", Optional: true}}, Cardinality: Many, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "device", "get"}, Summary: "Read one device.", Description: "Return a compact device record for DEVICE_ID without changing it. It includes identity, owner, authorization, and tags, but omits low-value API fields.", Response: "One compact device record.", Arguments: []Argument{{Name: "DEVICE_ID", Description: "Tailscale device ID.", Optional: false}}, Options: []Option{{Name: "--details", Description: "Include addresses, client version, expiry, and node ID.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "device", "route", "list"}, Summary: "List a device's subnet and exit-node routes.", Description: "Return route settings advertised by DEVICE_ID.", Response: "A collection of route records.", Arguments: []Argument{{Name: "DEVICE_ID", Description: "Tailscale device ID.", Optional: false}}, Cardinality: Many, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Update, Path: []string{"tailscale", "device", "name", "set"}, Summary: "Set a device name.", Description: "Rename DEVICE_ID to NAME. This changes device metadata but not its access policy.", Response: "Confirmation of the requested device name.", Arguments: []Argument{{Name: "DEVICE_ID", Description: "Tailscale device ID.", Optional: false}, {Name: "NAME", Description: "New device name.", Optional: false}}, Cardinality: One, Reversible: true, Reversal: "Record the current name with device get, then run the same name set command with that prior value.", Credentials: "tailscale", UnattendedOK: false},
	{Effect: Authorize, Path: []string{"tailscale", "device", "tag", "set"}, Summary: "Replace a device's tag set.", Description: "Replace all tags on DEVICE_ID with TAG values. Tags affect device identity and tailnet ACLs, so this is an authorization operation.", Response: "Confirmation of the requested tag set.", Arguments: []Argument{{Name: "DEVICE_ID", Description: "Tailscale device ID.", Optional: false}, {Name: "TAG", Description: "One or more tag:NAME values that replace the current tag set.", Optional: false}}, Cardinality: One, Reversible: true, Reversal: "Record the current tags with device get, then run the same tag set command with that exact prior tag set.", Credentials: "tailscale", UnattendedOK: false},
	{Effect: Observe, Path: []string{"tailscale", "acl", "get"}, Summary: "Download the tailnet ACL into a local file.", Description: "Write the HuJSON ACL to a new 0600 file. It is never printed to stdout and never overwrites an existing file.", Response: "The path of the downloaded ACL file and its ETag when supplied.", Options: []Option{{Name: "--output", Value: "PATH", Description: "New destination file; defaults to a timestamped acl.hujson file in the current directory.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "acl", "validate"}, Summary: "Validate a proposed tailnet ACL without applying it.", Description: "Send ACL_FILE to Tailscale's validation endpoint. The active ACL is not changed; policy tests in the file are evaluated when present. A rejected ACL returns a nonzero exit status.", Response: "A stable valid or invalid status and Tailscale's validation result.", Arguments: []Argument{{Name: "ACL_FILE", Description: "Path to a HuJSON tailnet ACL file to validate.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "acl", "preview"}, Summary: "Show ACL rules that match one source identity.", Description: "Preview the active ACL for SOURCE, or a candidate ACL supplied with --file, without applying any change. SOURCE can be a user or a tag such as tag:github.", Response: "The matching rules, destinations, and relevant posture conditions.", Options: []Option{{Name: "--for", Value: "SOURCE", Description: "User or tag identity to preview, for example tag:github.", Optional: false}, {Name: "--file", Value: "ACL_FILE", Description: "Candidate HuJSON ACL file; defaults to the active tailnet ACL.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Administer, Path: []string{"tailscale", "acl", "set"}, Summary: "Replace the tailnet ACL from a local file with a required backup.", Description: "Validate ACL_FILE, write the current HuJSON ACL to a new private BACKUP_FILE, then replace it only if its ETag is still current. The backup can be used to undo the change.", Response: "Confirmation with the backup path and ETag used for the guarded replacement.", Arguments: []Argument{{Name: "ACL_FILE", Description: "Path to a HuJSON tailnet ACL file.", Optional: false}}, Options: []Option{{Name: "--backup", Value: "BACKUP_FILE", Description: "Required new path for a 0600 snapshot of the current ACL; Omni never overwrites it.", Optional: false}}, Cardinality: One, Reversible: true, Reversal: "The required pre-change backup can be supplied to a later guarded acl set operation; Omni has no separate revert command.", Credentials: "tailscale", UnattendedOK: false},
	{Effect: Observe, Path: []string{"tailscale", "key", "list"}, Summary: "List accessible active Tailscale key metadata.", Description: "Return active key IDs and non-secret metadata, including key type, scopes, tags, expiry, status, and auth-key creation properties when available. Use --all to broaden active key types visible to the credential. It never returns key material.", Response: "A collection of compact active key metadata records.", Options: []Option{{Name: "--all", Description: "Request all visible active key types; the provider does not enumerate revoked keys in list results.", Optional: true}}, Cardinality: Many, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "key", "get"}, Summary: "Read non-secret metadata for one Tailscale key.", Description: "Return key type, scopes, tags, expiry, and invalid or revoked state for KEY_ID when the credential is authorized to read it. It never returns key material.", Response: "One compact key metadata record.", Arguments: []Argument{{Name: "KEY_ID", Description: "Tailscale control-plane key ID, for example k...CNTRL.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "dns", "get"}, Summary: "Read tailnet DNS configuration.", Description: "Return MagicDNS preferences, global nameservers, search paths, and split-DNS configuration without changing them.", Response: "A compact snapshot of tailnet DNS settings.", Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "user", "list"}, Summary: "List users in the configured tailnet.", Description: "Return compact user identities and account state.", Response: "A collection of compact user records.", Cardinality: Many, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"tailscale", "user", "get"}, Summary: "Read one user.", Description: "Return a compact account record for USER_ID without changing it.", Response: "One compact user record.", Arguments: []Argument{{Name: "USER_ID", Description: "Tailscale user ID.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "tailscale", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "board", "list"}, Summary: "List boards visible to the configured Trello user.", Description: "Discover Trello boards available to the configured user before selecting a board ID.", Response: "A collection of board IDs and names.", Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "board", "overview"}, Summary: "Read a board and its lists with card counts.", Description: "Return a selected board and its lists. Uses trello.default-board-id when BOARD_ID is omitted.", Response: "A board record and collection of list records.", Arguments: []Argument{{Name: "BOARD_ID", Description: "Trello board ID. Optional when trello.default-board-id is configured.", Optional: true}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "list", "list"}, Summary: "List the lists on one board.", Description: "Return list records for BOARD_ID or the configured default board.", Response: "A collection of Trello list records.", Arguments: []Argument{{Name: "BOARD_ID", Description: "Trello board ID. Optional when trello.default-board-id is configured.", Optional: true}}, Cardinality: Many, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Observe, Path: []string{"trello", "card", "get"}, Summary: "Read one Trello card.", Description: "Return the Trello API record for one card without changing it.", Response: "One Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}}, Cardinality: One, Reversible: true, Credentials: "trello", UnattendedOK: true},
	{Effect: Create, Path: []string{"trello", "card", "create"}, Summary: "Create one Trello card.", Description: "Create a card in LIST_ID with a required name and optional description or due date.", Response: "The created Trello card record.", Arguments: []Argument{{Name: "LIST_ID", Description: "Trello list ID that receives the card.", Optional: false}}, Options: []Option{{Name: "--name", Value: "NAME", Description: "Card title.", Optional: false}, {Name: "--description", Value: "TEXT", Description: "Card description.", Optional: true}, {Name: "--due", Value: "DATE", Description: "Card due date accepted by Trello.", Optional: true}}, Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false},
	{Effect: Move, Path: []string{"trello", "card", "move"}, Summary: "Move one Trello card to a list or position.", Description: "Move CARD_ID to LIST_ID. This changes workflow placement but keeps the card.", Response: "The updated Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}, {Name: "LIST_ID", Description: "Destination Trello list ID.", Optional: false}}, Options: []Option{{Name: "--position", Value: "POSITION", Description: "Destination position; defaults to bottom.", Optional: true}}, Cardinality: One, Reversible: true, Reversal: "Record the prior list and position, then run the same move command with those prior values.", Credentials: "trello", UnattendedOK: false},
	{Effect: Archive, Path: []string{"trello", "card", "archive"}, Summary: "Archive one Trello card.", Description: "Close one card while preserving it in Trello's archive.", Response: "The archived Trello card record.", Arguments: []Argument{{Name: "CARD_ID", Description: "Trello card ID.", Optional: false}}, Cardinality: One, Reversible: false, Credentials: "trello", UnattendedOK: false},
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
