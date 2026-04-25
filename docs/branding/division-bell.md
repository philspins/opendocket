# Division Bell — Brand Brief

**Version:** 1.0  
**Status:** Draft  
**Last updated:** April 2026

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Name & Origin](#2-name--origin)
3. [Mission & Positioning](#3-mission--positioning)
4. [Audience](#4-audience)
5. [Voice & Tone](#5-voice--tone)
6. [Taglines](#6-taglines)
7. [Visual Identity](#7-visual-identity)
8. [Typography](#8-typography)
9. [Colour Palette](#9-colour-palette)
10. [Iconography](#10-iconography)
11. [Motion & Animation](#11-motion--animation)
12. [Layout & UX Principles](#12-layout--ux-principles)
13. [Reference Sites](#13-reference-sites)
14. [What Division Bell Is Not](#14-what-division-bell-is-not)
15. [Trademark Notes](#15-trademark-notes)

---

## 1. Project Overview

**Division Bell** (`divisionbell.ca`) is a Canadian civic transparency platform that aggregates publicly available parliamentary and legislative data — bills, votes, MP profiles, campaign donations — and surfaces it in a clean, searchable, accountable format. It covers federal Parliament and all ten provincial legislatures.

The platform is built on open government data. No proprietary sources. No paywalls. No advertising.

It is the successor project to a civic tech effort previously operating under the name *Open Democracy* (`open-democracy.ca`).

**GitHub:** `github.com/philspins/open-democracy`  
**Tech stack:** Go · Templ · Alpine.js · Tailwind · SQLite → Postgres · Claude API

---

## 2. Name & Origin

### The parliamentary term

A **division bell** is the bell rung inside and around Parliament buildings to summon members for a recorded vote — a *division*. When the bell rings, every MP must stop what they are doing, return to the chamber, and be counted. It is one of the most direct expressions of parliamentary accountability: the moment when a politician must publicly take a side.

The name was chosen because it captures exactly what this platform does — it calls attention to those moments, and keeps the permanent record of how every representative answered.

### The cultural connection

*The Division Bell* is also the title of Pink Floyd's 1994 album. The phrase was coined for that album by the author Douglas Adams, who drew it from its parliamentary meaning. This gives the name a secondary cultural layer — recognisable, memorable, and carrying connotations of communication, accountability, and things left unsaid. It is not a liability; it makes the name stickier.

> **On trademark:** "Division Bell" as a standalone phrase in the context of a civic technology website is not registered as a trademark in Canada. Pink Floyd (1987) Ltd. holds the PINK FLOYD® trademark, not a claim on the underlying parliamentary term. The phrase's pre-existing parliamentary meaning, its coinage by Douglas Adams (not the band), and the entirely different commercial class (civic software vs. recorded music) make infringement claims extremely unlikely. If clarity is ever needed, the About page framing — *"named for the bell rung in Parliament to call members to a vote"* — is sufficient. See [Section 15](#15-trademark-notes) for full trademark notes.

---

## 3. Mission & Positioning

### Mission statement

> To make Canadian parliamentary democracy legible, searchable, and accountable — for every citizen, in every riding, at every level of government.

### What the platform does

- Tracks every bill through every stage of the legislative process, federally and provincially
- Records every recorded vote and links it to every member who cast one
- Summarises legislation in plain English (and French) using AI, with Library of Parliament summaries preferred where available
- Allows constituents to follow their representatives and receive digests of their voting activity
- Maps campaign donation data to donor industries and organisations, surfacing potential conflicts of interest through a **Loyalty Gauge** — a tri-axis indicator showing whether a politician's votes align most closely with their party, their donors, or the public
- Provides a `mailto:`-based constituent feedback tool so citizens can contact their representatives directly

### Positioning

Division Bell sits at the intersection of **civic journalism** and **civic technology**. It is not a news outlet — it does not editorialize. It is not a government portal — it is independent. It is not a political advocacy tool — it is non-partisan by design.

The closest analogy is a **primary source made navigable**. Hansard exists. LEGISinfo exists. ourcommons.ca exists. Division Bell makes them useful together.

---

## 4. Audience

### Primary

- Engaged Canadian citizens who follow political news and want to verify what they read
- Journalists and researchers who need structured parliamentary data
- Advocacy organisations tracking specific bill categories (housing, environment, Indigenous rights, etc.)
- Students of Canadian politics and civics

### Secondary

- Politicians and their staff (tracking how colleagues vote)
- Academics studying legislative behaviour
- Donors and civil society organisations interested in transparency

### Who this is *not* for (by design)

Division Bell does not chase casual or disengaged users. The platform assumes a baseline level of civic interest. It does not simplify to the point of distortion. It respects the intelligence of its audience.

---

## 5. Voice & Tone

### Core voice attributes

| Attribute | Description |
|-----------|-------------|
| **Authoritative** | Speaks with confidence about facts. Never hedges on what the record shows. |
| **Neutral** | Does not editorialize about politicians, parties, or policies. The data speaks. |
| **Precise** | Uses the correct parliamentary terminology. "Division," not "vote." "Royal Assent," not "signed into law." |
| **Direct** | Short sentences. Active voice. No filler. |
| **Accessible** | Parliamentary language is explained where needed, but not condescended to. |

### Tone by context

| Context | Tone |
|---------|------|
| Bill summaries | Plain English. Clear. Non-partisan. Like a good encyclopedia entry. |
| Vote records | Factual. Neutral. The record, presented without comment. |
| Loyalty Gauge | Clinical. Statistical. Always caveated: correlation, not causation. |
| Error states | Direct and helpful. Never cute or apologetic. |
| Empty states | Honest. "No data available" is better than a placeholder. |
| Onboarding | Brief and purposeful. Assume the user knows why they're here. |

### What Division Bell never sounds like

- Partisan or ideologically aligned in any direction
- Sensationalist ("SHOCKING vote reveals...")
- Corporate or promotional
- Sycophantic or user-flattering
- Vague or hedged when the record is clear

---

## 6. Taglines

### Primary tagline

> **They vote in your name. See for yourself.**

### Alternates

| Use case | Tagline |
|----------|---------|
| Homepage hero (short) | *Know how they voted.* |
| About / mission framing | *Democracy needs witnesses.* |
| Subheading / descriptor | *Every vote. Every member. Every time.* |
| Urgency / accountability | *The bell is ringing. Are you listening?* |
| Data / record framing | *The official record — made readable.* |

### French companion (bilingual hero)

> *La cloche sonne. Ils doivent répondre.*  
> The bell rings. They must answer.

This works particularly well as a split bilingual treatment on the homepage — English on one side, French on the other, separated by the bell mark.

---

## 7. Visual Identity

### Direction

**Serious / institutional.** The visual language of a watchdog organisation — a newspaper of record, not a startup. Credibility is communicated through restraint, precision, and density of useful information. Nothing bounces. Nothing pleads for attention.

The nearest cultural reference points are:
- A well-designed front page of a broadsheet newspaper
- The visual language of parliamentary publications and Hansard
- Editorial data journalism (ProPublica, The Markup)

### Overall aesthetic

- High contrast. Dark navy on off-white, or white on near-black.
- Typography-led. The hierarchy of information is established through type, not colour or decoration.
- Data as content. Tables, vote records, and bill progress indicators are treated as first-class visual elements, not afterthoughts.
- No photography. No illustrations. No stock imagery. The site lives in data, type, and iconography.
- Generous whitespace around data clusters. Density where it matters, breathing room around it.

---

## 8. Typography

### Type pairing

| Role | Typeface | Rationale |
|------|----------|-----------|
| **Display / Headlines** | *Libre Baskerville* | Editorial serif. Feels like a newspaper of record. Carries institutional weight without feeling governmental. |
| **Body / UI** | *DM Sans* | Humanist sans. Highly readable at small sizes. Pairs cleanly with the serif display face. |
| **Data / tables / code** | *Inconsolata* | Monospace signals precision in the data layer. Used for vote tallies, bill IDs, division numbers. |

### Type principles

- Establish hierarchy through weight and size, not colour
- Bill titles set in the display serif at larger sizes; vote metadata set in the mono
- Line lengths capped around 70 characters for body text — readable, editorial
- No decorative or novelty typefaces anywhere in the UI

---

## 9. Colour Palette

### Core palette

| Role | Hex | Name | Usage |
|------|-----|------|-------|
| **Primary** | `#1B3A5C` | Parliamentary Navy | Primary brand colour. Headers, primary buttons, the bell icon. |
| **Secondary** | `#4A6FA5` | Steel Blue | Links, interactive accents, hover states. |
| **Neutral Dark** | `#1A1A1A` | Near Black | Body text, primary headings. |
| **Neutral Mid** | `#6B7280` | Slate Grey | Secondary text, metadata, timestamps. |
| **Neutral Light** | `#F4F4F5` | Off White | Page background. Avoids the clinical feel of pure white. |
| **Surface** | `#FFFFFF` | White | Card backgrounds, modals, data panels. |
| **Alert / Accent** | `#C41E3A` | Parliamentary Red | Used sparingly: flagged votes, accountability alerts, donor alignment warnings. |
| **Positive** | `#2D6A4F` | Dark Green | Positive alignment indicators, bills that passed with public support. |

### Colour principles

- The red is used the way a newspaper uses it: rarely, so it carries weight when it appears
- Never use colour to communicate party affiliation — that path leads to the site reading as partisan
- The navy and grey palette should dominate; the red and green appear only in data contexts where a status must be communicated

### Dark mode palette

| Role | Hex | Name | Usage |
|------|-----|------|-------|
| **Background** | `#0D1117` | Midnight Ink | Page background — deep near-black with a blue undertone. Not pure black. |
| **Surface 1** | `#161D27` | Deep Slate | Card backgrounds, primary panels, sidebars. |
| **Surface 2** | `#1E2A3A` | Raised Slate | Elevated cards, modals, dropdown menus, hover states on Surface 1. |
| **Border** | `#243447` | Navy Divide | All borders, dividers, table rules. Visible but quiet. |
| **Primary** | `#2A5298` | Muted Navy | Primary buttons, active states — the light-mode navy, brightened for dark contexts. |
| **Primary Bright** | `#5B8DD9` | Clarity Blue | Links, interactive highlights, focus rings — readable against all dark surfaces. |
| **Text Primary** | `#E8E4DC` | Parchment | Primary body text and headings — warm off-white, not harsh pure white. |
| **Text Secondary** | `#9BA3AF` | Warm Grey | Secondary text, metadata, vote dates, timestamps. |
| **Text Muted** | `#6B7280` | Faded Slate | Disabled states, placeholder text, footnotes. |
| **Alert / Accent** | `#E03E55` | Tempered Red | Flagged votes, accountability alerts — slightly brighter than light-mode red for dark contrast. |
| **Alert Surface** | `#2A1018` | Red Tint | Background tint for alert/flagged rows in vote tables. |
| **Positive** | `#3D9970` | Forest | Positive alignment, bills passed with public support — brighter than light-mode green. |
| **Positive Surface** | `#0D2018` | Green Tint | Background tint for positive alignment rows. |
| **Data Mono** | `#7BAFD4` | Code Slate | Monospace data: vote tallies, bill IDs, division numbers — cool, clinical, readable. |

### Dark mode design notes

- **Background is not pure black.** Midnight Ink (`#0D1117`) has a blue undertone that keeps it in family with the parliamentary navy without feeling cold or harsh. Pure black makes the navy accents look purple.
- **Three surface levels.** Background → Surface 1 → Surface 2 creates depth for cards, modals, and hover states without introducing a fourth colour. Each step is subtle — if you can see a strong difference, it's too much contrast.
- **Text is warm, not white.** Parchment (`#E8E4DC`) carries the warmth of the light-mode off-white into dark mode. Pure white text on dark backgrounds causes eye strain on long reading sessions — important for a data-heavy site.
- **Accents are slightly brightened.** Both the red and green are ~10% brighter in dark mode than light mode. Dark backgrounds absorb colour; the light-mode values would look muddy without this adjustment.
- **Links use Clarity Blue, not Primary Navy.** The dark-mode navy (`#2A5298`) is used for primary buttons only. Links use Clarity Blue (`#5B8DD9`), which has enough luminance contrast against all three surface levels to pass WCAG AA without a heavy underline.
- **Data Mono (Code Slate).** A cool blue-grey (`#7BAFD4`) for monospace data — bill IDs, division numbers, vote tallies. Distinct from both the link colour and body text, which helps users scan data tables quickly.

---

## 10. Iconography

### The bell mark

The primary brand icon is a **minimal, geometric bell silhouette**:

- Symmetrical. Wider at the base than a church bell — closer to the actual shape of a parliamentary division bell.
- No ornamentation. No shadows. No gradients.
- Works at 16×16px (favicon) without losing its shape. This constraint should drive the design — if it doesn't read at favicon scale, it's too complex.
- Monochrome primary: navy on white, or white on navy. Can be used in red for alert contexts.
- A thin horizontal rule beneath the bell — suggesting the parliamentary bar, a ledger line, or the ruled lines of Hansard — is an optional compositional element in larger applications (wordmark, About page).

### System icons

- Use a consistent, minimal icon set throughout the UI — no mixing of styles
- Line icons preferred over filled, except for active/selected states
- Custom icons should only be created for Division Bell-specific concepts (the bell, the loyalty gauge needle, the division vote indicator)

---

## 11. Motion & Animation

### Principles

Less is more. Every animation must earn its place. The site should feel like it is revealing information, not performing for the user.

### Permitted animations

| Element | Animation | Notes |
|---------|-----------|-------|
| **The bell icon** | A single, slow toll (one gentle swing arc, ~600ms, ease-in-out) on new division events | Used very sparingly — on page load if a new vote has been recorded, or on the notification digest. Never on hover. |
| **Loyalty gauge needle** | Smooth sweep to its computed position on page load (~800ms, ease-out) | The one moment of deliberate drama on the MP profile page. |
| **Vote tallies** | Count up from zero to final number on first render (~400ms) | Signals that the number is live data, not static content. |
| **Bill stage progress** | Stages fill left to right as the page loads (~400ms staggered delay per stage) | Communicates sequence and progress without distraction. |
| **Page transitions** | Fade only (~150ms). No slides, no transforms. | |

### Prohibited

- Animations that loop indefinitely
- Hover animations on data tables (distracting when scanning rows)
- Loading spinners where skeleton screens can be used instead
- Anything that bounces, springs, or draws attention to itself

---

## 12. Layout & UX Principles

### Information density

Division Bell should feel **dense but readable** — like a well-designed broadsheet, not a billboard. Users who come to this site want information. Give it to them.

- The homepage leads with *Today in Parliament*: what happened, who voted, what passed. No hero image. No marketing copy above the fold.
- Tables are first-class layout elements — designed to be scanned, not hidden behind accordions
- Bill detail pages show stage timelines, vote breakdowns, and AI summaries in a structured hierarchy

### Navigation principles

- URL structure is canonical and deep-linkable. Every bill, every MP, every vote gets a stable, shareable URL.
- Search is prominent. This is a reference tool; users will arrive knowing what they want.
- No dark patterns. No email capture popups. No cookie banners beyond what is legally required.

### Mobile

The data-dense layout requires careful mobile consideration:

- Tables collapse to a card-per-row format on small screens
- The loyalty gauge reflows to a vertical bar comparison on narrow screens — the tri-axis gauge is not practical at mobile width
- Vote history tables are horizontally scrollable on mobile — do not collapse columns; preserve the full data

### Accessibility

- WCAG 2.1 AA minimum throughout
- All data tables have appropriate `scope` and `aria-label` attributes
- The loyalty gauge needle position is communicated via `aria-valuenow` and a text fallback
- Colour is never the sole means of communicating status — always paired with an icon or label
- The site must be fully functional without JavaScript for core read-only content (bills, votes, member profiles)

---

## 13. Reference Sites

Sites that occupy a similar aesthetic and mission territory — for designer reference, not copying:

| Site | What to take from it |
|------|---------------------|
| **ProPublica** (propublica.org) | Data journalism that takes itself seriously without being cold. Clean editorial hierarchy. |
| **The Markup** (themarkup.org) | Civic tech. High contrast. Very text-led. Strong typographic hierarchy. |
| **Open Secrets** (opensecrets.org) | Closest mission analogue (US campaign finance tracker). Data presentation model is instructive even if the design is aging. |
| **Hansard Society** (hansardsociety.org.uk) | Institutional and measured. Good example of a watchdog org's visual language. |
| **The Economist** | Mastery of data visualisation within an editorial frame. The benchmark for charts in a serious publication. |

**What all of these share:** None of them look like a startup. That is intentional and should guide every design decision for Division Bell.

---

## 14. What Division Bell Is Not

It is useful to be explicit about what this brand rejects:

| Not this | Because |
|----------|---------|
| A government portal | Must feel independent, not official |
| A political party's website | Must be visibly non-partisan |
| A tech startup | Credibility comes from restraint, not personality |
| A news outlet | Presents the record; does not editorialize |
| An advocacy tool | Non-partisan by design; presents all parties equally |
| A social network | No feeds, no likes, no virality mechanics |
| A consumer app | The audience is engaged citizens, not casual users |

---

## 15. Trademark Notes

> **This section is informational only and does not constitute legal advice. Consult a Canadian IP lawyer before filing any trademark applications.**

### "Division Bell" as a phrase

**Pink Floyd (1987) Ltd.** holds the registered trademark **PINK FLOYD®**. The phrase "The Division Bell" or "Division Bell" does not appear to be separately registered as a trademark in Canada in any class related to software, websites, or online information services.

Several factors make a successful trademark infringement claim against Division Bell (the platform) extremely unlikely:

- "Division bell" is a **pre-existing parliamentary term** with centuries of usage in Westminster-style legislatures, predating the album by hundreds of years
- The album title *The Division Bell* was coined by **Douglas Adams**, not by Pink Floyd — the band adopted a pre-existing phrase rather than creating it
- The commercial classes are entirely different: recorded music (Nice Class 9) vs. civic software and online information services (Nice Class 42)
- There is no reasonable consumer confusion between a civic transparency platform and a rock album

### Recommended precautions

1. **Register `divisionbell.ca` immediately** if not already done
2. **Consider a CIPO trademark application** for DIVISION BELL in Nice Class 42 (software as a service / online information services) — this establishes priority and makes the platform's rights explicit
3. **Include a brief contextual note on the About page:** *"Division Bell takes its name from the bell rung in Parliament to summon members for a recorded vote — a division."* This grounds the name in its parliamentary meaning without disclaiming the cultural resonance.
4. **Do not use Pink Floyd imagery**, album artwork, or song titles anywhere in the platform

### Prior name: "Open Democracy" / `open-democracy.ca`

The prior name *Open Democracy* was retired due to proximity to an existing organisation operating at `opendemocracy.ca`. The domain `open-democracy.ca` remains registered and should be held and redirected to `divisionbell.ca` once the new domain is active. No further action is required regarding the name.
