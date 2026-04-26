# Open Docket — Brand Brief

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
14. [What Open Docket Is Not](#14-what-open-docket-is-not)
15. [Domain & Trademark Notes](#15-domain--trademark-notes)

---

## 1. Project Overview

**Open Docket** (`opendocket.ca`) is a government transparency platform that aggregates publicly available parliamentary and legislative data — bills, votes, member profiles, and campaign donations — and surfaces it in a clean, searchable, and accountable format. It launches covering federal Parliament and all ten Canadian provincial legislatures, with architecture designed for global expansion to any Westminster-style democracy.

The platform is built entirely on open government data. No proprietary sources. No paywalls. No advertising.

It is the successor project to a civic tech effort previously developed under the working name *Open Democracy* (`open-democracy.ca`).

**GitHub:** `github.com/philspins/open-democracy`  
**Tech stack:** Go · Templ · Alpine.js · Tailwind · SQLite → Postgres · Claude API

---

## 2. Name & Origin

### What a docket is

A **docket** is the official record of matters scheduled to come before a court, a legislature, or a governing body. It is the agenda of power — the authoritative list of what is being decided, by whom, and when. In legal and parliamentary tradition, to enter something on the docket is to make it a matter of formal public record.

The word carries centuries of institutional weight. It appears in courtrooms, in Parliament, in legislative assemblies around the world. It is formal without being obscure. A 13-year-old may not know its precise legal definition, but will correctly infer that a docket is something official and important.

### Why "Open"

**Open** does the transparency work. An open docket is one the public can see — nothing hidden, nothing redacted, nothing buried in a PDF behind a government portal. The combination implies both the *content* (the official record of government business) and the *principle* (it belongs to everyone).

Together, **Open Docket** makes a quiet promise: *everything that happens in your name, visible to you.*

### Why this name was chosen

The name emerged from an extended search through the civic tech naming space — a search that eliminated dozens of candidates for being too obscure (*Division Bell*), too generic (*GovInsight*, *CivicLens*), too taken (*GovWatch*, *OpenSecrets*, *GovTrack*, *NorthWatch*), or too narrow (*VoteWatch*, *BillWatch*). The consistent failure of obvious names revealed that the available namespace for descriptive civic tech names is exhausted.

Open Docket works because it does not describe the platform's features — it describes the platform's *purpose*. It is not a tracker, a lens, or a watcher. It is the record itself, made open.

A Google exact-phrase search for "open docket" at the time of naming returned no established brand in this space — a rare result after months of searching.

---

## 3. Mission & Positioning

### Mission statement

> To put the official record of government into the hands of the people it belongs to.

### What the platform does

- Tracks every bill through every stage of the legislative process, federally and provincially across Canada — and eventually in any Westminster-style legislature globally
- Records every recorded division vote and links it to every member who cast one
- Summarises legislation in plain English using AI, with Library of Parliament summaries preferred where available
- Allows constituents to follow their representatives and receive weekly digests of their voting activity
- Maps campaign donation data to donor industries and organisations, surfacing potential conflicts of interest through the **Loyalty Gauge** — a tri-axis indicator showing whether a politician's votes align most closely with their party, their donors, or the public
- Provides a `mailto:`-based constituent feedback tool so citizens can contact their representatives directly, with no intermediary

### Positioning

Open Docket sits at the intersection of **civic journalism** and **civic technology**. It is not a news outlet — it does not editorialize. It is not a government portal — it is independent. It is not a political advocacy tool — it is non-partisan by design.

The closest analogy is a **primary source made navigable**. Hansard exists. LEGISinfo exists. ourcommons.ca exists. Open Docket makes them useful together, in one place, in plain language.

### Global ambition

The platform is designed from the outset for replication beyond Canada. The word "docket" is used in parliamentary and legal contexts across Canada, the United States, the United Kingdom, Australia, New Zealand, India, and Ireland. The name carries no country-specific baggage. When Open Docket expands to cover Westminster parliaments in other countries, the brand travels with it.

### Nearest competitor

The closest existing product is **GovWatch** (`govwatch.app`), a US-focused platform tracking Congress with nearly identical features: bills, votes, member profiles, campaign finance, plain-English summaries, and misconduct tracking. GovWatch validates the model. Open Docket is its Canadian counterpart — and its eventual global successor.

---

## 4. Audience

### Primary

- Engaged citizens who follow political news and want to verify what they read against the actual record
- Journalists and researchers who need structured parliamentary data without navigating government portals
- Advocacy organisations tracking specific bill categories (housing, environment, Indigenous rights, labour, etc.)
- Students of civics, political science, and Canadian government

### Secondary

- Politicians and their staff tracking how colleagues vote
- Academics studying legislative behaviour and political accountability
- Civil society organisations and NGOs interested in government transparency

### Who this is *not* for (by design)

Open Docket does not chase casual or disengaged users. The platform assumes a baseline level of civic interest. It respects the intelligence of its audience and does not simplify to the point of distortion. It is a reference tool, not an entertainment product.

---

## 5. Voice & Tone

### Core voice attributes

| Attribute | Description |
|-----------|-------------|
| **Authoritative** | Speaks with confidence about what the record shows. Never hedges on documented facts. |
| **Neutral** | Does not editorialize about politicians, parties, or policies. The data speaks. |
| **Precise** | Uses correct parliamentary and legal terminology. "Division," not "vote." "Royal Assent," not "signed into law." "Member," not "politician" where precision matters. |
| **Direct** | Short sentences. Active voice. No filler. No throat-clearing. |
| **Accessible** | Parliamentary and legal language is explained where needed, but never condescended to. |

### Tone by context

| Context | Tone |
|---------|------|
| Bill summaries | Plain English. Clear. Non-partisan. Like a well-written encyclopedia entry. |
| Vote records | Factual. Neutral. The record, presented without comment. |
| Loyalty Gauge | Clinical. Statistical. Always caveated: correlation, not causation. |
| Error states | Direct and helpful. Never cute or apologetic. |
| Empty states | Honest. "No data available" is better than a misleading placeholder. |
| Onboarding | Brief and purposeful. Assume the user knows why they are here. |
| Donation data | Careful. Factual. Never implies wrongdoing without documented evidence. |

### What Open Docket never sounds like

- Partisan or ideologically aligned in any direction
- Sensationalist ("SHOCKING vote reveals...")
- Corporate, promotional, or self-congratulatory
- Sycophantic or user-flattering
- Vague or hedged when the record is unambiguous
- Accusatory when presenting donor alignment data — correlation is not causation and the platform never implies otherwise

---

## 6. Taglines

### Primary tagline

> **The official record. Made open.**

### Alternates

| Use case | Tagline |
|----------|---------|
| Homepage hero (short) | *Everything on the docket. Nothing hidden.* |
| About / mission framing | *Democracy needs witnesses.* |
| Subheading / descriptor | *Every vote. Every member. Every time.* |
| Constituent empowerment | *They vote in your name. See for yourself.* |
| Data / record framing | *The record is public. Now it's readable.* |
| Accountability framing | *What they said. What they did. On the record.* |

### French companion (bilingual hero)

> *Le dossier est ouvert. Rien n'est caché.*  
> The docket is open. Nothing is hidden.

This works as a split bilingual treatment on the homepage or About page — the two languages side by side, separated by the docket mark, reinforcing that the platform serves all Canadians equally.

---

## 7. Visual Identity

### Direction

**Serious / institutional.** The visual language of a court of record or a newspaper of record — not a startup, not a government portal, not a political campaign. Credibility is communicated through restraint, precision, and the density of useful information. Nothing bounces. Nothing pleads for attention.

### The conceptual anchor

A docket is a **physical document** — stamped, ruled, numbered, filed. The visual identity draws from that tradition: the aesthetic of official records, court filings, Hansard pages, and ledger books. This is not retro or nostalgic; it is authoritative. The design language says: *this is where the real information lives.*

Concretely this means:
- Strong typographic hierarchy modelled on legal documents and broadsheet newspapers
- Ruled lines used purposefully — as dividers, table rules, and structural elements
- A stamp or seal motif for the wordmark, suggesting official certification
- Monospace type in data contexts, signalling precision and machine-readable accuracy
- No photography, no illustration, no decorative imagery — the site lives in data, type, and iconography

### Overall aesthetic

- High contrast — near-black on off-white in light mode, warm off-white on deep ink in dark mode
- Typography-led — hierarchy established through type weight and size, not colour
- Data as content — tables, vote records, bill progress indicators, and donation breakdowns are first-class visual elements
- Generous whitespace around data clusters; density where it matters, breathing room around it
- The restraint is the point — a site that looks like it is trying hard to impress you is not a site you trust with serious civic information

---

## 8. Typography

### Type pairing

| Role | Typeface | Rationale |
|------|----------|-----------|
| **Display / Headlines** | *Playfair Display* or *Libre Baskerville* | Editorial serif. Feels like a newspaper of record or a court document. Carries institutional weight without feeling governmental or bureaucratic. |
| **Body / UI** | *Source Sans 3* or *DM Sans* | Humanist sans. Highly readable at small sizes. Pairs cleanly with the serif display face without competing with it. |
| **Data / tables / monospace** | *JetBrains Mono* or *IBM Plex Mono* | Monospace signals precision in the data layer. Used for vote tallies, bill IDs, division numbers, member IDs, donation amounts, and docket numbers. |

### Type principles

- Hierarchy established through weight and size — not colour
- Bill titles and member names set in the display serif; vote metadata and identifiers in the mono
- Line lengths capped at approximately 70 characters for body text — readable, editorial
- No decorative or novelty typefaces anywhere in the UI
- All-caps used sparingly and only in the monospace for labels and status indicators — never in the serif

---

## 9. Colour Palette

### Light mode

| Role | Hex | Name | Usage |
|------|-----|------|-------|
| **Primary** | `#1B3A5C` | Parliamentary Navy | Primary brand colour. Headers, primary buttons, the docket mark. |
| **Secondary** | `#4A6FA5` | Steel Blue | Links, interactive accents, hover states. |
| **Neutral Dark** | `#1A1A1A` | Near Black | Body text, primary headings. |
| **Neutral Mid** | `#6B7280` | Slate Grey | Secondary text, metadata, timestamps, docket numbers. |
| **Neutral Light** | `#F4F4F5` | Off White | Page background. Avoids the clinical feel of pure white. |
| **Surface** | `#FFFFFF` | White | Card backgrounds, modals, data panels. |
| **Alert / Accent** | `#C41E3A` | Record Red | Used sparingly: flagged votes, accountability alerts, donor alignment indicators. |
| **Positive** | `#2D6A4F` | Dark Green | Positive alignment indicators, bills that passed with public support. |

### Dark mode

| Role | Hex | Name | Usage |
|------|-----|------|-------|
| **Background** | `#0D1117` | Midnight Ink | Page background — deep near-black with a blue undertone. Not pure black. |
| **Surface 1** | `#161D27` | Deep Slate | Card backgrounds, primary panels, sidebars. |
| **Surface 2** | `#1E2A3A` | Raised Slate | Elevated cards, modals, dropdowns, hover states on Surface 1. |
| **Border** | `#243447` | Navy Divide | All borders, dividers, table rules. Visible but quiet. |
| **Primary** | `#2A5298` | Muted Navy | Primary buttons, active states. |
| **Primary Bright** | `#5B8DD9` | Clarity Blue | Links, interactive highlights, focus rings. |
| **Text Primary** | `#E8E4DC` | Parchment | Primary body text and headings — warm off-white, not harsh pure white. |
| **Text Secondary** | `#9BA3AF` | Warm Grey | Secondary text, metadata, vote dates, timestamps. |
| **Text Muted** | `#6B7280` | Faded Slate | Disabled states, placeholder text, footnotes. |
| **Alert / Accent** | `#E03E55` | Tempered Red | Flagged votes, accountability alerts — slightly brighter than light-mode red for dark contrast. |
| **Alert Surface** | `#2A1018` | Red Tint | Background tint for flagged rows in vote and donation tables. |
| **Positive** | `#3D9970` | Forest | Positive alignment indicators — brighter than light-mode green for dark contrast. |
| **Positive Surface** | `#0D2018` | Green Tint | Background tint for positive alignment rows. |
| **Data Mono** | `#7BAFD4` | Code Slate | Monospace data: vote tallies, bill IDs, docket numbers — cool, clinical, readable. |

### Colour principles

- The red is used the way a newspaper uses it: rarely, so it carries weight when it appears
- Never use colour to communicate party affiliation — that path leads to the platform reading as partisan
- The navy and grey palette dominates; red and green appear only in data contexts where a status must be clearly communicated
- Background is `#0D1117`, not `#000000` — the blue undertone keeps dark mode in family with the parliamentary navy; pure black makes navy accents read as purple
- Text is `#E8E4DC` (Parchment), not white — a data-heavy site means long reading sessions; pure white on dark causes eye strain

---

## 10. Iconography

### The docket mark

The primary brand icon is a **minimal document stamp or seal**:

- A square or rectangular outline — suggesting a filed document, a rubber stamp, or a docket number block
- The letters **OD** or the word **DOCKET** set in the monospace typeface within the stamp boundary
- Optional: a thin diagonal rule across one corner, suggesting a filed or certified document
- No ornamentation beyond the geometric border and letterform
- Works at 16×16px (favicon) without losing legibility — this constraint drives the design
- Monochrome primary: navy on white, or white on navy. Used in red for alert contexts only.

**Alternative direction:** A simple open folder or open file icon — the docket as a folder of records, opened to the public. Simpler, more universally understood, slightly less distinctive. Worth exploring alongside the stamp concept.

### System icons

- Use a consistent, minimal line icon set throughout the UI — no mixing of styles or weights
- Line icons preferred over filled, except for active or selected states
- Custom icons reserved for Open Docket-specific concepts: the docket mark, the loyalty gauge, the division vote indicator, the bill stage progress pip

---

## 11. Motion & Animation

### Principles

Less is more. Every animation must earn its place. The site should feel like it is revealing information, not performing for the user. The aesthetic reference is a document being stamped and filed — deliberate, precise, final.

### Permitted animations

| Element | Animation | Notes |
|---------|-----------|-------|
| **Docket stamp** | A single downward press (~300ms, ease-in then sharp stop) when a new division is recorded | Used very sparingly — not on hover, not on page load by default. Only when a significant new event has been logged. |
| **Loyalty gauge needle** | Smooth sweep to its computed position on page load (~800ms, ease-out) | The one moment of deliberate drama on the member profile page. |
| **Vote tallies** | Count up from zero to final number on first render (~400ms) | Signals that the number is live data, not static content. |
| **Bill stage progress** | Stages fill left to right as the page loads (~400ms staggered delay per stage) | Communicates sequence and progress without distraction. |
| **Page transitions** | Fade only (~150ms). No slides, no transforms. | |
| **Table row reveal** | Rows fade in sequentially on first load (~50ms stagger per row, max 10 rows animated) | Suggests data being retrieved from a filing system. |

### Prohibited

- Animations that loop indefinitely
- Hover animations on data tables — distracting when scanning rows
- Loading spinners where skeleton screens can be used instead
- Anything that bounces, springs, or draws attention to itself
- Parallax effects — inconsistent with the serious institutional aesthetic

---

## 12. Layout & UX Principles

### Information density

Open Docket should feel **dense but readable** — like a well-designed broadsheet or a well-organised court document, not a billboard or a landing page. Users who come to this site want information. Give it to them efficiently.

- The homepage leads with *Today on the Docket*: what bills moved, who voted, what passed. No hero image. No marketing copy above the fold.
- Tables are first-class layout elements — designed to be scanned, sorted, and filtered, not hidden behind accordions
- Bill detail pages show stage timelines, vote breakdowns, and AI summaries in a clear, consistent hierarchy
- Member profile pages lead with the Loyalty Gauge, then vote history, then donation summary, then contact tools

### Navigation principles

- URL structure is canonical and deep-linkable. Every bill, every member, every vote, every donation record gets a stable, shareable URL.
- Search is prominent. This is a reference tool; most users will arrive knowing what they want to look up.
- No dark patterns. No email capture popups. No cookie banners beyond what PIPEDA legally requires.
- The platform must function fully without JavaScript for all core read-only content — bills, votes, member profiles, donation records.

### Mobile

The data-dense layout requires careful mobile consideration:

- Tables collapse to a card-per-row format on small screens, preserving all key fields
- The Loyalty Gauge tri-axis needle display reflows to a vertical bar comparison on narrow screens — the circular gauge is not practical at mobile width
- Vote history tables are horizontally scrollable on mobile — columns are not collapsed; the full record is always accessible
- Donation industry breakdown panels stack vertically on mobile

### Accessibility

- WCAG 2.1 AA minimum throughout — WCAG 2.2 AA as a target
- All data tables have appropriate `scope`, `headers`, and `aria-label` attributes
- The Loyalty Gauge needle position is communicated via `aria-valuenow` and a text fallback — the gauge is never the sole means of conveying the score
- Colour is never the sole means of communicating status — always paired with an icon or text label
- Focus states are visible and styled consistently — never removed
- The docket stamp animation respects `prefers-reduced-motion`

---

## 13. Reference Sites

Sites that occupy a similar aesthetic and mission territory — for designer reference, not copying:

| Site | What to take from it |
|------|---------------------|
| **GovWatch** (govwatch.app) | Closest functional equivalent — US Congress tracker with identical feature set. Useful for UX patterns even if the visual design diverges. |
| **ProPublica** (propublica.org) | Data journalism that takes itself seriously without being cold. Clean editorial hierarchy. Strong typographic discipline. |
| **The Markup** (themarkup.org) | Civic tech aesthetic. High contrast. Text-led. Exemplary typographic hierarchy. |
| **Open Secrets** (opensecrets.org) | Closest mission analogue for the donation tracking feature (US campaign finance). Data presentation model is instructive even if the design is aging. |
| **Hansard Society** (hansardsociety.org.uk) | Institutional and measured. Good example of a watchdog organisation's visual language. |
| **The Economist** | Benchmark for data visualisation within a serious editorial frame. The standard for charts in a publication that takes its readers seriously. |
| **PACER** (pacer.gov) | The US federal court records system — not a visual reference (it's famously ugly) but a *conceptual* reference: a docket system that the public can access. Open Docket is what PACER should look like. |

**What all of these share:** None of them look like a startup. That is intentional and should guide every design decision for Open Docket.

---

## 14. What Open Docket Is Not

It is useful to be explicit about what this brand rejects:

| Not this | Because |
|----------|---------|
| A government portal | Must feel independent, not official or bureaucratic |
| A political party's website | Must be visibly and provably non-partisan |
| A tech startup | Credibility comes from restraint and accuracy, not personality or growth metrics |
| A news outlet | Presents the record; does not editorialize or interpret |
| An advocacy tool | Non-partisan by design; all parties and members are presented equally |
| A social network | No feeds, no likes, no sharing mechanics, no virality |
| A consumer app | The audience is engaged citizens, not casual users to be retained |
| An accusatory platform | Donor alignment data shows correlation only — the platform never implies corruption without documented evidence |

---

## 15. Domain & Trademark Notes

> **This section is informational only and does not constitute legal advice. Consult a Canadian IP lawyer before filing any trademark applications or making domain acquisition offers.**

### Domain status

| Domain | Status | Action |
|--------|--------|--------|
| `opendocket.ca` | **Available** — register immediately | Register now. This is the primary domain. |
| `opendocket.com` | Registered since 2000 by Lexitas Legal (Houston, TX) — parked, no active site. Listed for sale via DomainAgents. | Make a modest offer ($300–800 USD). Not urgent — acquire over time. |
| `opendocket.org` | Registered May 2025 on Namecheap — likely a speculator. Expires May 2026. | Make a lowball offer ($100–200) via DomainAgents, or set a backorder for the expiry date. |

**Priority:** Register `opendocket.ca` today. The `.com` and `.org` are nice-to-have, not must-have for launch.

### "Open Docket" as a trademark

The phrase "open docket" does not appear to be registered as a trademark in Canada in any class related to software, websites, or online information services. A Google exact-phrase search returned no established brand using the name in this space.

**Recommended actions:**

1. **Register `opendocket.ca` immediately** — priority date matters for any future trademark claim
2. **Consider a CIPO trademark application** for OPEN DOCKET in Nice Class 42 (software as a service / online information services) once the platform has launched and established use — this formalises rights and deters future conflicts
3. **Check the US USPTO** for any existing OPEN DOCKET trademark registrations before acquiring `opendocket.com` — the Lexitas Legal connection suggests a legal services context; confirm no conflicting registration exists

### Prior names

The following names were considered and retired during development:

| Name | Reason retired |
|------|---------------|
| *Open Democracy* | Proximity to existing organisation at `opendemocracy.ca` |
| *Division Bell* | Failed family/general audience test — required explanation |
| *GovInsight* | Active EU consultancy at `govinsight.eu`; `.com` parked by unrelated party |
| *NorthWatch* | Taken by an environmental protection organisation |
| *GovWatch* | Taken by a near-identical US platform at `govwatch.app` |

The domain `open-democracy.ca` remains registered and should be held and redirected to `opendocket.ca` once the new domain is active.
