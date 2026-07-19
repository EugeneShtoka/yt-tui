# ANSI Styling in bubbles/table Cells

## The Problem

`github.com/charmbracelet/bubbles/table` v0.21.0 truncates cell content using
`github.com/mattn/go-runewidth` v0.0.19:

```go
// table.go line 429
renderedCell := m.styles.Cell.Render(
    style.Render(runewidth.Truncate(value, m.cols[i].Width, "…")))
```

`runewidth.Truncate` is **not ANSI-aware**. It treats ESC (0x1B) as zero-width
(it's a control char < 0x20) but counts every other byte in an escape sequence
as a visible character:

| Sequence      | runewidth |
|---------------|-----------|
| `\033[2m`     | 3 (`[2m`) |
| `\033[m`      | 2 (`[m`)  |
| `\033[0m`     | 3 (`[0m`) |
| `\033[22;39m` | 7         |

If a styled cell's **runewidth(ANSI open) + runewidth(content) + runewidth(ANSI close)**
exceeds the column width, truncation drops the trailing reset code. The terminal
is left with the attribute (e.g. Dim/faint) active and it **bleeds into every
subsequent cell and row**.

## The Fix

Shorten the *visible* content before applying ANSI so that:

```
runewidth(ANSI_open) + runewidth(content) + runewidth(ANSI_close) ≤ colWidth
```

Measure the overhead with a one-character probe — **not** `style.Render("")`
because lipgloss skips ANSI codes for empty strings:

```go
// Correct: use "X" to capture real ANSI sequences
overhead := runewidth.StringWidth(swapReset(style.Render("X"))) - 1
safeWidth := colWidth - overhead
```

For the Dim style (`\033[2m…\033[22;39m` after swapReset) the overhead is 10.

## swapReset

The table applies `styles.Selected.Render(row)` to the *entire selected row*.
That style adds a background color. A full `\033[0m` reset mid-cell clears the
background for the rest of the row.

Replace the trailing full reset with a partial one that preserves background:

```go
func swapReset(s string) string {
    const partial = "\033[22;39m"   // clears bold/faint + foreground, keeps background
    if strings.HasSuffix(s, "\033[0m") {
        return s[:len(s)-4] + partial
    }
    if strings.HasSuffix(s, "\033[m") {
        return s[:len(s)-3] + partial
    }
    return s
}
```

`\033[22;39m` runewidth = 7 (must be included in the overhead calculation above).

## Column Width Constraints

With 10 runewidth of overhead, a column must be **at least 11 chars wide** to
show even a single visible character with Dim styling. Practical limits:

| Column      | Width | Safe content | Feasible? |
|-------------|-------|--------------|-----------|
| Title       | ~80   | ~70          | ✓ (primary use case) |
| Channel     | 30    | 20           | ✓ (names truncated at 20) |
| Duration    | 19    | 9            | ✗ (DurationWithPos fills the column) |
| Views       | 8     | −2           | ✗ |
| Date        | 11    | 1            | ✗ |

## Usage Pattern

```go
// Compute once per render pass, outside the row loop.
dimOverhead := runewidth.StringWidth(swapReset(styles.Dim.Render("X"))) - 1

// For a left-aligned column (e.g. channel, 30 wide):
safeWidth := render.ColChannel - dimOverhead   // = 20
cell := swapReset(styles.Dim.Render(render.Truncate(rawValue, safeWidth)))

// For the title (uses titleSafeWidth which accounts for style-specific overhead):
func titleSafeWidth(titleW int, style lipgloss.Style) int {
    overhead := runewidth.StringWidth(swapReset(style.Render("X"))) - 1
    w := titleW - overhead
    if w < 1 { w = 1 }
    return w
}
title := styledTitle(v.Title, style, titleSafeWidth(titleW, style))

// Right-aligned columns that fill their width cannot be safely styled this way.
// Leave them unstyled or expand the column width by dimOverhead.
```

## What Does Not Work

- **`runewidth.Truncate` is called by the library before your content reaches
  `styles.Cell.Render`** — you cannot intercept it.
- Applying ANSI to a pre-padded string (`ralign(dur, ColDuration)`) that already
  fills the column will always cause truncation and bleed.
- `style.Render("")` returns `""` in lipgloss (empty input → no ANSI output),
  so it cannot be used to measure ANSI overhead.
- Per-row cell styles are not supported by bubbles/table; only a single `Cell`
  style applies to all rows uniformly.
