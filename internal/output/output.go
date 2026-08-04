// Package output renders Omni's canonical response values for people or
// machine consumers. JSON is the stable structured representation; text is a
// presentation derived from the same value.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

type Format string

const (
	Text Format = "text"
	JSON Format = "json"
)

func Parse(value string) (Format, error) {
	switch strings.ToLower(value) {
	case "text":
		return Text, nil
	case "json":
		return JSON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q; use text or json", value)
	}
}

func Encode(w io.Writer, format Format, value any) error {
	if format == JSON {
		return json.NewEncoder(w).Encode(value)
	}
	if format != Text {
		return fmt.Errorf("unsupported output format %q", format)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return err
	}
	renderer := textRenderer{w: w}
	return renderer.root(normalized)
}

type textRenderer struct{ w io.Writer }

func (r textRenderer) root(value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		_, err := fmt.Fprintln(r.w, scalar(value))
		return err
	}
	keys := sortedKeys(object)
	for i, key := range keys {
		if i > 0 {
			if _, err := fmt.Fprintln(r.w); err != nil {
				return err
			}
		}
		if err := r.section(key, object[key], ""); err != nil {
			return err
		}
	}
	return nil
}

func (r textRenderer) section(name string, value any, indent string) error {
	switch typed := value.(type) {
	case []any:
		if _, err := fmt.Fprintf(r.w, "%s%s (%d)\n", indent, heading(name), len(typed)); err != nil {
			return err
		}
		return r.list(typed, indent+"  ")
	case map[string]any:
		if _, err := fmt.Fprintf(r.w, "%s%s\n", indent, heading(name)); err != nil {
			return err
		}
		return r.object(typed, indent+"  ")
	default:
		_, err := fmt.Fprintf(r.w, "%s%s: %s\n", indent, heading(name), field(value))
		return err
	}
}

func (r textRenderer) object(object map[string]any, indent string) error {
	for _, key := range orderedKeys(object) {
		if err := r.section(key, object[key], indent); err != nil {
			return err
		}
	}
	return nil
}

func (r textRenderer) list(values []any, indent string) error {
	if len(values) == 0 {
		_, err := fmt.Fprintln(r.w, indent+"(none)")
		return err
	}
	objects := make([]map[string]any, len(values))
	for i, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			for _, item := range values {
				if _, err := fmt.Fprintf(r.w, "%s- %s\n", indent, scalar(item)); err != nil {
					return err
				}
			}
			return nil
		}
		objects[i] = object
	}
	if cardRecords(objects) {
		return r.cards(objects, indent)
	}
	columns := listColumns(objects)
	writer := tabwriter.NewWriter(r.w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, strings.Join(upper(columns), "\t")); err != nil {
		return err
	}
	for _, object := range objects {
		row := make([]string, len(columns))
		for i, column := range columns {
			row[i] = field(object[column])
		}
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func (r textRenderer) cards(cards []map[string]any, indent string) error {
	columns := []string{"NAME", "PROGRESS", "LABELS", "MEMBERS", "DUE"}
	if anyCardHasList(cards) {
		columns = append(columns, "LIST")
	}
	writer := tabwriter.NewWriter(r.w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, indent+strings.Join(columns, "\t")); err != nil {
		return err
	}
	for _, card := range cards {
		row := []string{scalar(card["name"]), cardProgress(card), cardLabels(card), memberSummary(card), dueDate(card["due"])}
		if len(columns) == 6 {
			list, _ := card["list"].(map[string]any)
			row = append(row, scalar(list["name"]))
		}
		if _, err := fmt.Fprintln(writer, indent+strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func cardRecords(records []map[string]any) bool {
	return len(records) > 0 && records[0]["id"] != nil && records[0]["name"] != nil && records[0]["idList"] != nil
}

func anyCardHasList(cards []map[string]any) bool {
	for _, card := range cards {
		if _, ok := card["list"].(map[string]any); ok {
			return true
		}
	}
	return false
}

func cardProgress(card map[string]any) string {
	badges, _ := card["badges"].(map[string]any)
	total, _ := badges["checkItems"].(float64)
	done, _ := badges["checkItemsChecked"].(float64)
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f/%.0f", done, total)
}

func cardLabels(card map[string]any) string {
	labels, _ := card["labels"].([]any)
	names := make([]string, 0, len(labels))
	for _, value := range labels {
		if label, ok := value.(map[string]any); ok {
			if name := scalar(label["name"]); name != "" {
				names = append(names, name)
			} else if color := scalar(label["color"]); color != "" {
				names = append(names, color)
			}
		}
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

func memberSummary(card map[string]any) string {
	if members, ok := card["members"].([]any); ok && len(members) > 0 {
		initials := make([]string, 0, len(members))
		for _, value := range members {
			if member, ok := value.(map[string]any); ok {
				if initial := scalar(member["initials"]); initial != "" {
					initials = append(initials, initial)
				}
			}
		}
		if len(initials) > 0 {
			return strings.Join(initials, ", ")
		}
	}
	members, _ := card["idMembers"].([]any)
	if len(members) == 0 {
		return "-"
	}
	return fmt.Sprintf("%d assigned", len(members))
}

func dueDate(value any) string {
	due := scalar(value)
	if len(due) >= 10 {
		return due[:10]
	}
	return due
}

func listColumns(objects []map[string]any) []string {
	available := map[string]bool{}
	for _, object := range objects {
		for key := range object {
			available[key] = true
		}
	}
	priority := []string{"id", "name", "hostname", "username", "fullName", "os", "closed", "state", "due", "dueComplete", "lastSeen", "date", "pos", "color", "mimeType", "bytes", "url"}
	columns := make([]string, 0, 8)
	for _, key := range priority {
		if available[key] {
			columns = append(columns, key)
			delete(available, key)
		}
	}
	for _, key := range sortedKeys(available) {
		if len(columns) == 8 {
			break
		}
		columns = append(columns, key)
	}
	return columns
}

func orderedKeys(object map[string]any) []string {
	priority := []string{"id", "name", "status", "description", "closed", "state", "due", "dueComplete", "dateLastActivity", "shortUrl"}
	keys := make([]string, 0, len(object))
	seen := map[string]bool{}
	for _, key := range priority {
		if _, ok := object[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, key := range sortedKeys(object) {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	return keys
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func heading(value string) string { return strings.ToUpper(strings.ReplaceAll(value, "_", " ")) }

func upper(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = heading(value)
	}
	return result
}

// field renders one value for a person. A null or empty value becomes a visible
// placeholder: a colorless Trello label, an unset due date, or an empty
// description would otherwise print as a bare heading and read as a rendering
// failure rather than as a value. JSON keeps the original null.
func field(value any) string {
	if rendered := scalar(value); rendered != "" {
		return rendered
	}
	return "-"
}

func scalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.ReplaceAll(strings.ReplaceAll(typed, "\n", " "), "\t", " ")
	case []any:
		parts := make([]string, len(typed))
		for i, item := range typed {
			parts[i] = scalar(item)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		parts := make([]string, 0, len(typed))
		for _, key := range orderedKeys(typed) {
			parts = append(parts, key+"="+scalar(typed[key]))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(value)
	}
}
