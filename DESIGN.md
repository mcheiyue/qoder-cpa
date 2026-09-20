# Qoder CPA WebUI Design System

## 0. Research Log

- Internal management surface: operational account management, not a marketing page.
- CPA management API contract: authenticated management routes and an unauthenticated resource route for the embedded page.
- Browser QA: not available in the current environment because no Chromium executable is installed.

## 1. Direction

The page is a compact operational panel for scanning account state and changing one
transport setting. It keeps the first viewport useful: title, login action, status
feedback, and the account table are visible without a hero or nested cards.

## 2. Tokens

- Background: `#f5f7fb`; dark background `#101318`.
- Surface: `#ffffff`; dark surface `#191e27`.
- Text: `#182230`; dark text `#f2f4f7`.
- Muted text: `#667085`; dark muted text `#98a2b3`.
- Border: `#d9dee8`; dark border `#303846`.
- Primary action: `#2563eb` with white text.
- Radius: 6px for controls, 8px for the single table region.
- Spacing: 8px control rhythm, 16px section rhythm, 24px page inset.

## 3. Layout

- A centered content column is capped at 980px.
- The header contains the page title/context and the primary login action.
- The account table occupies the full content width and scrolls horizontally below 620px.
- No fixed or sticky panel may cover the table or status message.

## 4. Components

- Primary button: opens CPA's native Qoder OAuth URL in a new tab.
- Status region: a live region for login, save, and error feedback.
- Account table: label, email, status, profile selector, and save action.
- Empty state: one concise sentence with the login action remaining visible.

## 5. Interaction And Accessibility

- Native buttons and selects are keyboard reachable.
- Status feedback uses `role="status"`.
- Account values are HTML-escaped before insertion.
- Reduced-motion behavior is implicit because the page has no animation.

## 6. Responsive Behavior

- The page inset remains 24px on wide screens and naturally shrinks with the viewport.
- The table retains readable column widths through horizontal scrolling instead of clipping or overlaying content.
- Color scheme follows the operating system through `prefers-color-scheme`.

## 7. Accepted Debt

- Visual QA at 375px, 768px, and 1280px is pending a Chromium-capable environment.
- The page is intentionally a single embedded HTML resource; no frontend build tool or dependency is introduced.
