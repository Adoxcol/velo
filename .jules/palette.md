## 2025-02-19 - Dynamic State Updates for Icon-Only Buttons in Mini UI

**Learning:** When toggling state on icon-only buttons (such as `#pp` in Velo Mini), updating `element.title` alone does not announce the updated button function to screen readers; `aria-label` must be updated alongside `title`.
**Action:** Always update both `title` and `aria-label` simultaneously in JavaScript when an icon-only button changes state dynamically.

## 2025-03-05 - ARIA Labels on Unwrapped Settings Controls

**Learning:** In Velo's `.opt` setting rows, `<input type="checkbox">` and `<select>` elements are rendered inside `.switch` or `.row2` spans without `<label for="...">` associations or `aria-labelledby`, leaving screen readers with unlabelled inputs.
**Action:** Always provide explicit `aria-label` attributes corresponding to the setting title text for form inputs and switches in `.opt` rows.

## 2025-03-15 - ARIA Labeling on Modal Dialogs and Ambiguous Location Buttons

**Learning:** Native `<dialog>` elements in Velo require explicit `aria-labelledby` (pointing to the dialog title ID) or `aria-label` so screen readers announce dialog context upon opening. Additionally, generic text-only buttons like "Change" or "Reset" in location inputs need explicit `aria-label` attributes to clarify their action context.
**Action:** Ensure every `<dialog>` element has `aria-labelledby` or `aria-label`, and add explicit `aria-label`s to generic context action buttons.

## 2025-03-28 - Dynamic Confirmation States on Action Buttons

**Learning:** When an action button transitions to a multi-step confirmation state (such as "Delete partial file?"), updating visible button text alone leaves screen readers and tooltip hovers uninformed of the destructive confirmation context.
**Action:** Update both `aria-label` and `title` when entering a multi-step confirmation state on action buttons, and clear timer handles cleanly on subsequent clicks or timeouts.

## 2025-04-02 - ARIA Pressed Attributes on Filter Chip Toggles

**Learning:** Velo uses `.chip` button elements as interactive toggle filters for list categories, queues, and batch downloads. Without `aria-pressed="true|false"`, screen reader users cannot perceive which filter chip is currently active.
**Action:** Always include `aria-pressed="${isActive}"` on `.chip` filter buttons when rendering or updating category and list filters.

## 2025-05-10 - File Context in Download Card Action Buttons

**Learning:** Card action buttons (`Pause`, `Resume`, `Show in folder`, `Cancel`) rendered inside download lists are ambiguous to screen reader users if given generic ARIA labels without target item context.
**Action:** Include the file name in card action `aria-label`s (e.g. `Pause ${escapeHtml(fileName)}`) to ensure screen readers announce specific target context for each action.
