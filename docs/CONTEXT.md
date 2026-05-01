# OpenDocket Domain Glossary

Terms used in architecture discussions. When naming a module, use these terms — not generic substitutes.

---

## Visitor

A person browsing OpenDocket. May be **authenticated** (has a verified account and session cookie) or a **guest** (no account, but may have set their riding via cookie). Both cases are first-class: guests can see personalised representative data without logging in.

A Visitor carries:
- An optional authenticated `User` (nil for guests)
- A `FederalRidingID` (from the user's saved profile, or from a guest riding cookie)
- A `ProvincialRidingID` (same sources)

A Visitor does **not** carry resolved representative objects — those are fetched fresh at render time to reflect elections and by-elections.

**Package:** `internal/visitor`

---

## Member

An elected or appointed representative in a legislature — federal MP, Senator, or provincial MLA/MPP/MNA/etc. Identified by an internal `id` string. A Member belongs to a `Riding` and a `Party`, and has a `GovernmentLevel` ("federal" or "provincial").

---

## Riding

A geographic electoral district. Identified by a name string (e.g. "Vancouver Centre"). A Riding maps to at most one active federal Member and at most one active provincial Member at any given time.

---

## Bill

A piece of legislation introduced in a legislature. Has a lifecycle of `Stage`s. May be federal or provincial. Identified by a structured `id` (e.g. `C-56`, `on-123`, `pe-bill-14`).

---

## Division

A recorded vote in a legislature. A Division captures the Yea/Nay/Paired counts and the individual `MemberVote` records for each Member who participated. Federal divisions come from the House of Commons or Senate; provincial divisions come from province-specific legislature sources.

---

## Parliament / Session

Federal context: a **Parliament** is the period between elections; each Parliament has one or more **Sessions**. Provincial legislatures use **Legislature** and **Session** with the same meaning.

---

## Party Alignment

Whether a Member's vote on a Division matches the majority position of their Party on that Division. Used in member stats and vote history display. For a Member who is the **sole elected representative** of their party at a given government level (e.g. a Green Party leader with no other elected members), their vote **defines** the party position — so party alignment is not computed (it would always be 100% by definition).
