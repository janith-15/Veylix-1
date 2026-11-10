# VEYLIX VPN — Design Spacing System

**Type:** Living reference · **Owner rule:** one inset token, one scale — no ad-hoc numbers.
**Last verified:** 2026-10-23 — the shell matrix (§3) and the inset variables in §2 were re-checked against `frontend/src/components/layout/layout.css` (`--sidebar-offset` used at lines 20-22, 35, 54, 56, 74; `--space-*` used for all page/card padding). No drift found; no code change required.

---

## 1. The scale (4px base)

| Token | Value | Use |
|---|---|---|
| `--space-1` | 4px | icon↔label, badge padding |
| `--space-2` | 8px | inline groups, table cell gaps |
| `--space-3` | 12px | **shell inset**, button gaps |
| `--space-4` | 16px | card padding, grid gap, dense shell padding |
| `--space-5` | 20px | **default page padding** |
| `--space-6` | 24px | section rhythm, editorial page padding |
| `--space-8` | 32px | page-level separation |
| `--space-10` | 40px | hero blocks |

**Rule:** never write a raw px value for spacing in a component — pick the nearest token.

---

## 2. Shell inset — one variable, four roles

```css
--sidebar-offset: 0px;      /* edge | hud | ruled */
--sidebar-offset: 12px;     /* floating */
```

| Role | Property | Value |
|---|---|---|
| Sidebar inset | `.app-sidebar` `top / bottom / left` | `--sidebar-offset` |
| Topbar inset | `.app-topbar` `margin: top right` | `--sidebar-offset` |
| Content gutter | `.app-main` `margin-right` | `--sidebar-offset` |
| Content offset | `.app-main` `margin-left` | `calc(--sidebar-width + --sidebar-offset)` |

**Why:** the sidebar is `position: fixed`, so its offset cannot be part of a flex container.
Deriving the content offset from the *same* two variables makes drift impossible.

---

## 3. Per-shell matrix

| Shell | Presets | Sidebar inset | Topbar inset | Page padding | Identity |
|---|---|---|---|---|---|
| `floating` | **Modern** | 12px | 12px | **20px** | floaty, detached panels |
| `edge` | **Dark** | 0 | 0 | **20px** | full-bleed glass |
| `hud` | **Gaming** | 0 | 0 | **16px** | dense HUD |
| `ruled` | **Light** | 0 | 0 | **24px** | editorial, airy |

`Classic` also uses `edge` (solid admin panel, zero decoration).

---

## 4. Component spacing contract

| Component | Padding | Gap |
|---|---|---|
| `Card` | `--space-4` | header `--space-3` bottom |
| `Button` (md) | `8px 16px` | `--space-2` between icon+label |
| `Input` | `8px 12px` | label↔field `--space-1` |
| `Table` cell | `10px 14px` | — |
| `Badge` | `2px 9px` | `5px` |
| Dialog | `--space-6` | sections `--space-4` |
| PageHeader | 0 | `--space-4` to content |

---

## 5. Vertical rhythm

- Page sections: `--space-6` (24px) — `.dash { gap: var(--space-6) }`
- Grid: `--space-4` (16px)
- Inside a card: `--space-2` … `--space-4`

---

## 6. Responsive rules

| Breakpoint | Sidebar | Content offset | Page padding |
|---|---|---|---|
| ≥1024 | `--sidebar-width` 250 | `calc(width + offset)` | shell default |
| 640–1023 | **208** | `calc(width + offset)` | shell default |
| <640 | drawer (off-canvas) | 0 | `--space-4` |

**CSS custom properties must be read at the use site** (`calc()` inside the property),
never pre-computed into an intermediate variable on `:root` — that froze stale values
before `[data-shell]` could override them.

---

## 7. Anti-patterns (learned the hard way)

| ❌ Don't | ✅ Do |
|---|---|
| `margin-left` on main **and** `margin` on sidebar | one inset variable, both sides derived |
| `--main-offset` computed on `:root` | `calc()` at the property using the live vars |
| `padding-left` on `.app-shell` for a fixed sidebar | the padding shifts fixed children in some engines |
| `filter` / `transform` on `.app-shell` | creates a containing block → breaks `position: fixed` |
| `position: relative` in a generic `.glass` utility | overrides shell positioning → sidebar falls into flow |
| Any `@media` rule touching `.app-main` margin | only the shell blocks in `layout.css` own it |
