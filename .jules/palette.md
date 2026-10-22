## 2025-02-19 - Dynamic State Updates for Icon-Only Buttons in Mini UI

**Learning:** When toggling state on icon-only buttons (such as `#pp` in Velo Mini), updating `element.title` alone does not announce the updated button function to screen readers; `aria-label` must be updated alongside `title`.
**Action:** Always update both `title` and `aria-label` simultaneously in JavaScript when an icon-only button changes state dynamically.

## 2025-03-05 - ARIA Labels on Unwrapped Settings Controls

**Learning:** In Velo's `.opt` setting rows, `<input type="checkbox">` and `<select>` elements are rendered inside `.switch` or `.row2` spans without `<label for="...">` associations or `aria-labelledby`, leaving screen readers with unlabelled inputs.
**Action:** Always provide explicit `aria-label` attributes corresponding to the setting title text for form inputs and switches in `.opt` rows.
