package audit

import (
	"fmt"
	"sort"
	"strings"
)

// resourceTypeLabels maps raw resource_type strings to short Spanish labels
// used by the audit summary. Missing entries fall through to the raw
// resource_type verbatim — that's fine, the frontend keeps its own fallback.
var resourceTypeLabels = map[string]string{
	"deal":         "deal",
	"contact":      "contacto",
	"pipeline":     "pipeline",
	"stage":        "etapa",
	"team":         "equipo",
	"team_member":  "miembro",
	"bot_flow":     "flujo de bot",
	"invitation":   "invitación",
	"role":         "rol",
	"whatsapp_template": "plantilla de WhatsApp",
}

// BuildSummary generates a short human sentence describing an audit event.
// Spanish-only for now (audience is Venezuelan SMBs). If i18n becomes
// necessary, swap the hardcoded strings for message keys + resolver calls.
// before/after are the JSON-unmarshalled payloads from the DB (usually
// map[string]any); nil is tolerated on both sides.
func BuildSummary(action, resourceType string, before, after any) string {
	label := humanResourceType(resourceType)

	switch action {
	case ActionCreate:
		return fmt.Sprintf("Creó %s", label)
	case ActionDelete:
		return fmt.Sprintf("Eliminó %s", label)
	case ActionTransferOwnership:
		return "Transfirió la propiedad del equipo"
	case ActionUpdate:
		if field := firstChangedField(before, after); field != "" {
			return fmt.Sprintf("Actualizó %s: %s", label, field)
		}
		return fmt.Sprintf("Actualizó %s", label)
	default:
		// Unknown action — surface the raw verb rather than lying.
		if action == "" {
			return ""
		}
		return fmt.Sprintf("%s %s", titleCase(action), label)
	}
}

func humanResourceType(rt string) string {
	if label, ok := resourceTypeLabels[rt]; ok {
		return label
	}
	return rt
}

// titleCase capitalizes only the first rune of s (ASCII fallback enough for
// our known action constants like "CUSTOM_ACTION").
func titleCase(s string) string {
	lower := strings.ToLower(s)
	if lower == "" {
		return lower
	}
	return strings.ToUpper(lower[:1]) + lower[1:]
}

// firstChangedField returns the first key (in sorted order) that differs
// between before and after when both are map[string]any. Sorted order keeps
// the summary deterministic across re-renders. Returns "" if inputs are not
// maps or no scalar field differs.
func firstChangedField(before, after any) string {
	b, okB := before.(map[string]any)
	a, okA := after.(map[string]any)
	if !okA {
		return ""
	}

	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// Skip metadata-ish keys: timestamps and ids rarely carry meaning for
		// the user reading "what changed".
		if k == "updated_at" || k == "created_at" || k == "id" {
			continue
		}
		var prev any
		if okB {
			prev = b[k]
		}
		if !scalarEquals(prev, a[k]) {
			return k
		}
	}
	return ""
}

// scalarEquals compares two decoded-JSON values "close enough" for change
// detection. Complex types (maps, slices) compare by fmt.Sprintf reflection,
// which is cheap and adequate — worst case we over-report changes.
func scalarEquals(x, y any) bool {
	if x == nil && y == nil {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	return fmt.Sprintf("%v", x) == fmt.Sprintf("%v", y)
}
