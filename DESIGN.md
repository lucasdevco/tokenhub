# tokenhub interface

The approved prototype establishes a restrained macOS utility style. The native
panel is compact and uses MyGo theme colors/system controls. Web settings carry
the same hierarchy: persistent navigation, service controls, labeled fields and
an explicit save/apply bar. Logs use the same React shell, palette and controls as settings, with a two-column history/inspector workspace.

## Tokens

- Body font: platform system sans serif; code/data use platform monospace.
- Web body text: 13px; page titles: 22px; section headings: 15px.
- Web chrome is compact: 52px topbar, 16–24px page insets, 10px form-row
  padding and 8px table-cell padding. Log filters share one desktop row and
  stack on compact screens. Request and response content expands naturally without boxes or nested
  scroll areas; the drawer itself scrolls. Existing actions and data remain visible.
- Web background and drawer: #ffffff; sidebar and secondary surfaces: #f8faff; content text: #233247.
- Secondary text: #58687c; separators: #eaf0f6; control borders: #dfe7ef.
- Blue action: #2563eb; hover #1d4ed8; selection #edf4ff with text #1e40af.
- White is the dominant canvas. Pale blue distinguishes navigation and hover states; the drawer backdrop uses a light tint to keep the workspace bright.
- Buttons and inputs: 6px radius; YAML editor: 8px. Shared controls use
  shadcn/ui focus rings in #3b82f6; navigation links retain a 3px outline
  with 2px offset.
- Field groups use rows and separators. Controls use native labels and states.

## Behavior

The standalone request examples document uses shared labeled controls, with an
entry in the logs page header and a return link instead of sidebar navigation.
Protocol, API base URL, model, prompt and streaming settings update a readable,
wrapping curl block with a copy action and explicit success/failure feedback.
Fields stack on compact screens.

Providers use a list plus detail form; at 850px or less the list becomes horizontal.
At 600px or less, settings navigation becomes horizontal above the content.
The save bar stays at the bottom of the viewport as its section scrolls. Dirty
drafts are protected on page unload. Browser and menu-bar status come from the same manager. No decorative
animation or external fonts/assets are required for this utility.

## Validation scope

Frontend tests and type/build checks, backend race tests, native compilation and
app packaging passed. Review permits shipping based on source inspection after
fixes. CUA screenshots are unavailable because the Node runtime failed to start;
this records the approved design and implementation without visual signoff.

## Native menu-bar panel

The native panel is normally 360×380 DIPs, with 14px insets and 10px section gaps. It grows by 32 DIPs for feedback and 30 DIPs for pending configuration, keeping footer actions visible.
Its first row contains equal-width, quiet Provider and Account pop-up buttons; only hover adds a background.
Labels are muted, long values truncate, and tooltips retain the full names.
Native menus show the current selection with a check and link directly to the
corresponding browser settings page. Provider switching immediately applies to
new requests; pending config changes disable it until restart.
The Account selector lists only the selected provider's accounts and displays its
remembered active account. Both selectors reuse the shared provider catalog and
version-checked ChangeProvider operation; selection applies immediately to new
requests without restarting or interrupting active requests. Providers without
credentials show No Account Required; an empty provider selection disables the
account control. Both management links open provider settings.

A raised service surface groups status and address. A compact status row carries the semantic dot, state label, and muted restart
and power icons with tooltips. A stopped service has a labeled start action.
The address remains selectable monospace text with a copy icon. Feedback uses a
single bounded two-line area with a full-message tooltip; pending changes expose
an explicit Restart and Apply action. Request Logs is the first menu row;
Program Runtime Log and Open Application Directory form a separate diagnostics
group with terminal and folder icons and explicit File/Finder destinations. The application directory is the active config file’s parent, containing config.yaml and app.log.
The request entry uses a blue icon on a pale blue well, a medium-weight title, and a separate surface. Diagnostics use smaller secondary text and plain rows. Settings moves to the footer alongside app identity and quit, separated by a subtle rule.

The opaque shell uses #f3f5f8 with white service and request surfaces, or #1c1f25 with #272c34 surfaces in dark mode. Primary text is #202b3b (dark: #edf1f7), and secondary text is #606d80 (dark: #a9b4c4). A restrained blue accent distinguishes the request entry, apply action and keyboard focus. Hover fills and separators reuse the same blue-gray family. The status dot retains semantic green/red. macOS popover vibrancy stays disabled.
Headless MyGo rendering verifies light/dark layouts, long selections, menus,
pending changes, stopped service and bounded error feedback; this is renderer
verification rather than a running desktop screenshot.

Finder actions in browser settings use the same native shell through protected
management endpoints. They are disabled outside the desktop shell. The caller
chooses a fixed directory kind, not an arbitrary file-system path. A log directory
is created if necessary on explicit open and follows the active capture directory.

## React request inspector

The logs surface uses a 42px topbar and an 18px page title without an introductory
paragraph. The request histogram has an 80px plot and reuses the shared palette
and segmented controls. List text is 14px, secondary text and column headings are
13px, and request rows use a 22px line height for readable, compact scanning.
Request paths use weight 500 and relative times weight 400. Absolute times and
byte sizes stay readable at 14px in muted text; methods and domains use muted
13px text, with methods at weight 400. Completed statuses remain neutral;
unfinished statuses use amber (#875b15), and failures use red (#aa3030) at weight
500, including their state labels.

History and request detail share one workspace. Above 800px, the list and
inspector are side by side; below 800px they stack. Chinese controls match the
settings page. Live-update pause and stream pause remain separate. The log
content has a bounded scroll region, safe text rendering, original-file links,
raw/hex pagination and metadata disclosure.

## Web components: shadcn/ui

Web controls use locally owned shadcn/ui components from the official `new-york`
registry, configured in `web/components.json`: Button, Input, NativeSelect,
Textarea, Label, Switch, Alert, Badge, Table, ToggleGroup and Sheet. This extends
the existing blue-and-white utility interface and preserves its compact layout
and functionality. Existing application CSS continues to own page layout,
responsive behavior and dense settings/logs presentation.

Tailwind CSS 4 semantic tokens map to the existing surface, text, blue action,
border and focus colors in `web/src/styles.css`; reused controls use body text
at 13px and small control text at 12px. Settings fields associate labels and help
text with controls; switches retain their controlled and disabled states.

The request inspector uses Sheet for modal focus management, Escape and backdrop
close behavior. It retains its remembered viewport-relative width, pointer
resize handle and keyboard resizing: left/right arrows adjust by 40px, Home
selects the minimum width and End fills the viewport. The minimum width is 360px,
bounded by the viewport; content continues to scroll inside the drawer.


### Lightweight control treatment

Inputs, selects and textareas use a near-white #fcfdff surface with a quiet
1px #dfe7ef border, darkening to #c3d0df on hover. Remove resting shadows from
controls, buttons, badges and the inspector Sheet; only the switch thumb keeps
a small functional shadow. Keyboard focus remains explicit through a blue border
and a 2px translucent ring on text controls. Preserve readable text and semantic
status colors rather than fading the whole interface.

Settings field rows use spacing instead of repeated horizontal rules. Keep subtle
#eaf0f6 dividers for structural boundaries and log-table scanning. Outlined actions
use the same subtle divider color; errors use a pale red border and tinted surface.
The Sheet uses a 6% foreground backdrop and brief 150–200ms functional transitions.

## Global provider selection

Settings pages show a labeled Current Provider selector above the editor. Selecting a saved provider immediately persists and switches new requests without changing the client URL or interrupting active streams. Unsaved drafts and pending config changes disable switching with an explanation. Provider editing and activation are separate. Empty selection returns a JSON 503; upstream errors pass through unchanged. Provider fields use base_url and preserve native client paths, including /v1.


## Provider accounts

The Services page uses a flat list with name, address and account summary.
The current service and account use app-owned popup menus; each service keeps
its own selection. Add opens a preset picker, then a dialog with name, address
and named accounts. API keys are concealed with a reveal action; headers,
templates and logging are under Advanced. Cancel protects unsaved changes.
The existing system font and compact controls are retained, with neutral gray
surfaces and an indigo action color on this page. At compact widths controls
stack and the dialog remains bounded by the viewport.

Services and accounts load from the protected management API. Save edits only
one service; save, delete and selection update persisted configuration and new
request targets without restarting or interrupting in-flight requests. Loading,
errors, saving and version conflicts retain the draft. Other pending config changes must be explicitly restarted before service changes.
The YAML editor remains under the collapsed Advanced navigation.
Account keys are literal values in the private config file; environment lookup,
variable references and environment profiles are removed. The catalog endpoint
requires a management session, including for reads.
