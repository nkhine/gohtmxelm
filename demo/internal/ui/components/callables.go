package components

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type InteractionExampleSpec struct {
	Slug        string
	Kind        string
	Category    string
	Title       string
	Description string
	ButtonLabel string
	Doc         string
	Snippet     string
}

func InteractionExampleSpecs() []InteractionExampleSpec {
	return []InteractionExampleSpec{
		{
			Slug:        "confirm-dialog",
			Kind:        "confirm-dialog",
			Category:    "Dialog",
			Title:       "Confirm dialog",
			Description: "Ask before a destructive action and return a yes/no result.",
			ButtonLabel: "Delete item",
			Doc:         "Render the dialog as a server fragment into a shared overlay root. The caller passes only a status target; the fragment owns the buttons and closes itself with a typed result.",
			Snippet:     "r.Get(\"/api/interactions/confirm\", renderConfirm)\n\n<button data-gohtmxelm-open=\"/api/interactions/confirm\" data-gohtmxelm-status=\"#result\">\n  Delete item\n</button>\n<div data-gohtmxelm-interactions-root></div>",
		},
		{
			Slug:        "alert-dialog",
			Kind:        "alert-dialog",
			Category:    "Dialog",
			Title:       "Alert dialog",
			Description: "Show a notice where acknowledgement is the whole result.",
			ButtonLabel: "Show alert",
			Doc:         "Use the same overlay root, but return only an acknowledgement string or status. This is useful when closing the panel is the value.",
			Snippet:     "templ Alert(target string) {\n  <div role=\"alertdialog\" data-gohtmxelm-status-target={ target }>\n    <p>Build completed.</p>\n    <button data-gohtmxelm-result=\"acknowledged\">OK</button>\n  </div>\n}",
		},
		{
			Slug:        "prompt-input",
			Kind:        "prompt-input",
			Category:    "Dialog",
			Title:       "Prompt for input",
			Description: "Collect text and return the submitted value or cancellation.",
			ButtonLabel: "Rename project",
			Doc:         "Keep transient input inside the fragment. On submit, read the input and close the overlay; no durable server state is required unless you post the result back.",
			Snippet:     "<input data-gohtmxelm-prompt-input value=\"gohtmxelm demo\" />\n<button data-gohtmxelm-prompt-submit>Save</button>\n<button data-gohtmxelm-result=\"cancelled\">Cancel</button>",
		},
		{
			Slug:        "nested-dialog",
			Kind:        "nested-dialog",
			Category:    "Dialog",
			Title:       "Nested dialog",
			Description: "Open another instance of the same server-rendered dialog.",
			ButtonLabel: "Open nested",
			Doc:         "Carry a depth or call ID in the fragment URL. Each returned fragment is appended to the overlay root, so every layer can close independently.",
			Snippet:     "func NextNestedRoute(target string, depth int) string {\n  return \"/api/interactions/nested?target=\" + target + \"&depth=\" + strconv.Itoa(depth+1)\n}",
		},
		{
			Slug:        "save-mutation",
			Kind:        "save-mutation",
			Category:    "Dialog",
			Title:       "Save form mutation flow",
			Description: "Validate on the server; keep the dialog open on error.",
			ButtonLabel: "Open form",
			Doc:         "Post the form with HTMX and swap only the panel. Validation errors return the same panel with field values preserved; success returns a done button that resolves the interaction.",
			Snippet:     "<form hx-post=\"/api/interactions/save\" hx-target=\"closest .call-panel\" hx-swap=\"outerHTML\">\n  <input name=\"name\" />\n  <button type=\"submit\">Save</button>\n</form>",
		},
		{
			Slug:        "account-dialog",
			Kind:        "account-dialog",
			Category:    "Dialog",
			Title:       "Account-aware dialog",
			Description: "Read root/session data when rendering the fragment.",
			ButtonLabel: "Open account dialog",
			Doc:         "Do not pass repeated session props from the button. Read them in the Go handler and render the fragment with the current account context.",
			Snippet:     "claims, ok := sso.Session(r)\nname := \"Guest\"\nif ok { name = claims.Name }\ncomponents.AccountDialog(target, name).Render(r.Context(), w)",
		},
		{
			Slug:        "optional-async",
			Kind:        "optional-async",
			Category:    "Dialog",
			Title:       "Confirm with optional async",
			Description: "Use one confirmation UI for immediate or delayed resolution.",
			ButtonLabel: "Open confirm",
			Doc:         "The immediate button closes directly. The async button disables itself, performs work, then closes with the final result.",
			Snippet:     "<button data-gohtmxelm-result=\"accepted\">Resolve now</button>\n<button data-gohtmxelm-async-result=\"accepted after work\">Run async</button>",
		},
		{
			Slug:        "progress-toast",
			Kind:        "progress-toast",
			Category:    "Notification",
			Title:       "Progress toast",
			Description: "Patch a singleton toast as work progresses.",
			ButtonLabel: "Start download",
			Doc:         "Keep a stable toast element ID and stream Datastar patch events from Go. Each progress update replaces the same element, so callers do not create duplicate toasts.",
			Snippet:     "stream.PatchElements(render(components.ProgressToast(\"Downloading... 40%\", 40, false)))\n\n<button data-on:click=\"@get('/api/interactions/progress-stream')\">Start</button>",
		},
		{
			Slug:        "error-banner",
			Kind:        "error-banner",
			Category:    "Notification",
			Title:       "Auto-dismissing error",
			Description: "Stack transient errors and remove them after a timeout.",
			ButtonLabel: "Show error",
			Doc:         "Create a new toast element for each event. Use a short timeout for automatic dismissal and a close button for manual dismissal.",
			Snippet:     "const toast = document.createElement('div')\ntoast.className = 'call-toast error'\nsetTimeout(() => toast.remove(), 3200)",
		},
		{
			Slug:        "live-status",
			Kind:        "live-status",
			Category:    "Notification",
			Title:       "Live status update",
			Description: "Update one pinned status pill from outside the component.",
			ButtonLabel: "Cycle status",
			Doc:         "Use a stable DOM ID and replace its text/class when status changes. This can be driven by SSE, a polling response, or local connection events.",
			Snippet:     "upsertToast(\"connection-status\", \"status\", \"Online: all views are current\")",
		},
		{
			Slug:        "upload-broadcast",
			Kind:        "upload-broadcast",
			Category:    "Notification",
			Title:       "Broadcast to every call",
			Description: "Apply one update to every open upload indicator.",
			ButtonLabel: "Start uploads",
			Doc:         "Render each upload with its own filename and a shared status class. A broadcast only flips the common status, leaving per-item data intact.",
			Snippet:     "document.querySelectorAll('.upload-pill').forEach((pill) => {\n  pill.classList.toggle('offline', !online)\n})",
		},
		{
			Slug:        "item-picker",
			Kind:        "item-picker",
			Category:    "Picker",
			Title:       "Item picker",
			Description: "Show a list and return the selected item.",
			ButtonLabel: "Pick item",
			Doc:         "Render a simple list of server-known choices. Each row closes the overlay with the selected value; cancellation returns a separate result.",
			Snippet:     "<button data-gohtmxelm-result=\"selected Invoice\">Invoice</button>\n<button data-gohtmxelm-result=\"selected Receipt\">Receipt</button>",
		},
		{
			Slug:        "color-picker",
			Kind:        "color-picker",
			Category:    "Picker",
			Title:       "Color picker",
			Description: "Resolve a swatch grid with a selected hex value.",
			ButtonLabel: "Pick color",
			Doc:         "Pass the current color as a prop or query value when needed. Each swatch is a normal button with the chosen value in a data attribute.",
			Snippet:     "<button style=\"background:#0ea5e9\" data-gohtmxelm-result=\"#0ea5e9\"></button>",
		},
		{
			Slug:        "context-menu",
			Kind:        "context-menu",
			Category:    "Menu",
			Title:       "Context menu",
			Description: "Open a positioned menu from a right-click target.",
			ButtonLabel: "Right click",
			Doc:         "Intercept the contextmenu event, forward client coordinates to Go, and render the menu with a fixed position.",
			Snippet:     "zone.addEventListener('contextmenu', (e) => {\n  e.preventDefault()\n  GoHTMXElmInteractions.open(`/api/interactions/menu?x=${e.clientX}&y=${e.clientY}`, '#result')\n})",
		},
		{
			Slug:        "command-palette",
			Kind:        "command-palette",
			Category:    "Menu",
			Title:       "Command palette",
			Description: "Search commands and resolve with Enter.",
			ButtonLabel: "Open palette",
			Doc:         "Render commands from Go, then keep keyboard focus and filtering local. For app-wide shortcuts, open the same fragment from a keydown handler.",
			Snippet:     "<input data-gohtmxelm-command-search />\n<button data-gohtmxelm-command-item data-gohtmxelm-result=\"ran Export data\">Export data</button>",
		},
		{
			Slug:        "bottom-sheet",
			Kind:        "bottom-sheet",
			Category:    "Drawer",
			Title:       "Bottom sheet",
			Description: "Render an action menu from the bottom edge.",
			ButtonLabel: "Open sheet",
			Doc:         "Use the same overlay root as a dialog, but align the panel to the bottom. The interaction contract remains a fragment plus a result.",
			Snippet:     "<div class=\"call-backdrop sheet\">\n  <div class=\"call-panel call-sheet-panel\">...</div>\n</div>",
		},
		{
			Slug:        "settings-drawer",
			Kind:        "settings-drawer",
			Category:    "Drawer",
			Title:       "Settings drawer",
			Description: "Edit local form state and return saved settings.",
			ButtonLabel: "Open settings",
			Doc:         "Render initial settings from Go. Let the drawer own edits until the user saves; then close with the selected values or post them to the server.",
			Snippet:     "<select data-gohtmxelm-settings-email><option>daily</option><option>weekly</option></select>\n<button data-gohtmxelm-settings-save>Save</button>",
		},
		{
			Slug:        "image-lightbox",
			Kind:        "image-lightbox",
			Category:    "Overlay",
			Title:       "Image lightbox",
			Description: "Open media in a full-screen overlay.",
			ButtonLabel: "Open lightbox",
			Doc:         "Use a fragment for the overlay shell and close it via backdrop click, Escape, or a close button. Real apps can pass the image URL in the fragment request.",
			Snippet:     "<button data-gohtmxelm-open=\"image-lightbox\" data-gohtmxelm-status=\"#result\">Open image</button>",
		},
		{
			Slug:        "wizard",
			Kind:        "wizard",
			Category:    "Flow",
			Title:       "Multi-step wizard",
			Description: "Keep a multi-step form inside one interaction.",
			ButtonLabel: "Sign up",
			Doc:         "Keep step state local to the opened panel. The caller only receives the final structured result or cancellation, so back/forward navigation stays inside the flow.",
			Snippet:     "<div data-gohtmxelm-wizard data-step=\"0\">\n  <section data-gohtmxelm-wizard-step=\"0\">...</section>\n  <button data-gohtmxelm-wizard-next>Next</button>\n</div>",
		},
		{
			Slug:        "permission-consent",
			Kind:        "permission-consent",
			Category:    "Flow",
			Title:       "Permission consent",
			Description: "Return a tagged allow/deny response.",
			ButtonLabel: "Ask permission",
			Doc:         "Prefer a tagged result when deny and allow carry domain meaning. It is clearer than treating everything as a boolean.",
			Snippet:     "<button data-gohtmxelm-result=\"deny:reportbot\">Deny</button>\n<button data-gohtmxelm-result=\"allow:reportbot\">Allow</button>",
		},
		{
			Slug:        "resolve-from-caller",
			Kind:        "resolve-from-caller",
			Category:    "Flow",
			Title:       "Resolve from caller",
			Description: "Settle a specific open interaction from outside.",
			ButtonLabel: "Open approval",
			Doc:         "Mark the opened panel with a call ID or data attribute. A timeout, SSE event, or parent workflow can find that panel and close it without an in-panel click.",
			Snippet:     "setTimeout(() => {\n  const panel = document.querySelector('[data-gohtmxelm-auto-resolve]')\n  if (panel) closeCallable(panel, 'external timeout resolved false')\n}, 3000)",
		},
	}
}

func FindInteractionExample(slug string) (InteractionExampleSpec, bool) {
	for _, ex := range InteractionExampleSpecs() {
		if ex.Slug == slug {
			return ex, true
		}
	}
	return InteractionExampleSpec{}, false
}

func StatusID(kind string) string {
	switch kind {
	case "error-banner":
		return "call-status-error"
	case "live-status":
		return "call-status-live"
	case "upload-broadcast":
		return "call-status-upload"
	}
	return "call-status-" + strings.TrimPrefix(kind, "call-")
}

func StatusSelector(kind string) string {
	return "#" + StatusID(kind)
}

// ContextMenuRoute adds cursor coordinates to the context-menu fragment URL.
func ContextMenuRoute(target string) string {
	q := url.Values{}
	q.Set("target", target)
	q.Set("x", "0")
	q.Set("y", "0")
	return "/api/callables/context-menu?" + q.Encode()
}

// CallableStyle returns a safe inline position for context menus.
func CallableStyle(x, y int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return fmt.Sprintf("left:%dpx;top:%dpx", x, y)
}

// NextNestedRoute advances the nested dialog stack.
func NextNestedRoute(target string, depth int) string {
	q := url.Values{}
	q.Set("target", target)
	q.Set("depth", strconv.Itoa(depth+1))
	return "/api/callables/nested-dialog?" + q.Encode()
}

func NestedClosed(depth int) string {
	return "closed nested #" + strconv.Itoa(depth)
}

func PercentStyle(percent int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return "width:" + strconv.Itoa(percent) + "%"
}

func CallableTarget(v string) string {
	if strings.HasPrefix(v, "#call-status-") {
		return v
	}
	return "#call-status-generic"
}
