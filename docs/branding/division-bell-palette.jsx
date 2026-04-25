import { useState } from "react";

const lightPalette = [
  { role: "Primary",        name: "Parliamentary Navy", hex: "#1B3A5C", textColor: "#fff", usage: "Headers, primary buttons, bell icon" },
  { role: "Secondary",      name: "Steel Blue",         hex: "#4A6FA5", textColor: "#fff", usage: "Links, interactive accents, hover states" },
  { role: "Neutral Dark",   name: "Near Black",         hex: "#1A1A1A", textColor: "#fff", usage: "Body text, primary headings" },
  { role: "Neutral Mid",    name: "Slate Grey",         hex: "#6B7280", textColor: "#fff", usage: "Secondary text, metadata, timestamps" },
  { role: "Neutral Light",  name: "Off White",          hex: "#F4F4F5", textColor: "#1A1A1A", usage: "Page background" },
  { role: "Surface",        name: "White",              hex: "#FFFFFF", textColor: "#1A1A1A", usage: "Card backgrounds, modals, data panels" },
  { role: "Alert / Accent", name: "Parliamentary Red",  hex: "#C41E3A", textColor: "#fff", usage: "Flagged votes, accountability alerts — use sparingly" },
  { role: "Positive",       name: "Dark Green",         hex: "#2D6A4F", textColor: "#fff", usage: "Positive alignment indicators" },
];

const darkPalette = [
  { role: "Background",        name: "Midnight Ink",       hex: "#0D1117", textColor: "#fff", usage: "Page background — deep, near-black with a blue undertone. Not pure black." },
  { role: "Surface 1",         name: "Deep Slate",         hex: "#161D27", textColor: "#fff", usage: "Card backgrounds, primary panels, sidebars" },
  { role: "Surface 2",         name: "Raised Slate",       hex: "#1E2A3A", textColor: "#fff", usage: "Elevated cards, modals, dropdown menus, hover states on Surface 1" },
  { role: "Border",            name: "Navy Divide",        hex: "#243447", textColor: "#fff", usage: "All borders, dividers, table rules. Visible but quiet." },
  { role: "Primary",           name: "Muted Navy",         hex: "#2A5298", textColor: "#fff", usage: "Primary buttons, active states — the light-mode navy, brightened for dark contexts" },
  { role: "Primary Bright",    name: "Clarity Blue",       hex: "#5B8DD9", textColor: "#0D1117", usage: "Links, interactive highlights, focus rings — readable against all dark surfaces" },
  { role: "Text Primary",      name: "Parchment",          hex: "#E8E4DC", textColor: "#1A1A1A", usage: "Primary body text and headings — warm off-white, not harsh pure white" },
  { role: "Text Secondary",    name: "Warm Grey",          hex: "#9BA3AF", textColor: "#1A1A1A", usage: "Secondary text, metadata, vote dates, timestamps" },
  { role: "Text Muted",        name: "Faded Slate",        hex: "#6B7280", textColor: "#fff", usage: "Disabled states, placeholder text, footnotes" },
  { role: "Alert / Accent",    name: "Tempered Red",       hex: "#E03E55", textColor: "#fff", usage: "Flagged votes, accountability alerts — slightly brighter than light-mode red for dark contrast" },
  { role: "Alert Surface",     name: "Red Tint",           hex: "#2A1018", textColor: "#fff", usage: "Background tint for alert/flagged rows in vote tables" },
  { role: "Positive",          name: "Forest",             hex: "#3D9970", textColor: "#fff", usage: "Positive alignment, bills passed with public support — brighter than light-mode green" },
  { role: "Positive Surface",  name: "Green Tint",         hex: "#0D2018", textColor: "#fff", usage: "Background tint for positive alignment rows" },
  { role: "Data Mono",         name: "Code Slate",         hex: "#7BAFD4", textColor: "#0D1117", usage: "Monospace data: vote tallies, bill IDs, division numbers — cool, clinical, readable" },
];

const darkInUse = [
  {
    label: "MP Profile — vote history table row",
    bg: "#161D27",
    border: "#243447",
    elements: [
      { label: "Bill number", color: "#5B8DD9", mono: true },
      { label: "Division description", color: "#E8E4DC", mono: false },
      { label: "YEA", color: "#3D9970", mono: false },
      { label: "Date", color: "#9BA3AF", mono: true },
    ]
  },
  {
    label: "Flagged vote — donor alignment",
    bg: "#2A1018",
    border: "#C41E3A44",
    elements: [
      { label: "Bill C-91", color: "#E03E55", mono: true },
      { label: "Oil Subsidies Act", color: "#E8E4DC", mono: false },
      { label: "YEA", color: "#E03E55", mono: false },
      { label: "Donors: SUPPORT", color: "#E03E55", mono: false },
    ]
  },
  {
    label: "Public-aligned vote",
    bg: "#0D2018",
    border: "#3D997044",
    elements: [
      { label: "Bill C-47", color: "#3D9970", mono: true },
      { label: "Housing Act", color: "#E8E4DC", mono: false },
      { label: "YEA", color: "#3D9970", mono: false },
      { label: "Public: 78% support", color: "#3D9970", mono: false },
    ]
  },
];

function Swatch({ color, mode }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    navigator.clipboard.writeText(color.hex);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div
      onClick={copy}
      style={{
        cursor: "pointer",
        borderRadius: "6px",
        overflow: "hidden",
        border: mode === "dark" ? "1px solid #243447" : "1px solid #e5e7eb",
        transition: "transform 0.1s",
      }}
      onMouseEnter={e => e.currentTarget.style.transform = "translateY(-2px)"}
      onMouseLeave={e => e.currentTarget.style.transform = "translateY(0)"}
    >
      {/* Colour block */}
      <div style={{
        background: color.hex,
        height: "72px",
        display: "flex",
        alignItems: "flex-end",
        padding: "8px",
      }}>
        <span style={{
          fontFamily: "monospace",
          fontSize: "11px",
          color: color.textColor,
          opacity: 0.8,
          background: "rgba(0,0,0,0.25)",
          padding: "2px 5px",
          borderRadius: "3px",
        }}>
          {copied ? "✓ copied" : color.hex}
        </span>
      </div>
      {/* Label */}
      <div style={{
        padding: "10px 12px",
        background: mode === "dark" ? "#161D27" : "#fff",
      }}>
        <div style={{
          fontSize: "10px",
          fontFamily: "monospace",
          color: mode === "dark" ? "#5B8DD9" : "#4A6FA5",
          letterSpacing: "0.1em",
          textTransform: "uppercase",
          marginBottom: "3px",
        }}>
          {color.role}
        </div>
        <div style={{
          fontSize: "13px",
          fontWeight: "600",
          color: mode === "dark" ? "#E8E4DC" : "#1A1A1A",
          marginBottom: "4px",
        }}>
          {color.name}
        </div>
        <div style={{
          fontSize: "11px",
          color: mode === "dark" ? "#9BA3AF" : "#6B7280",
          lineHeight: 1.4,
        }}>
          {color.usage}
        </div>
      </div>
    </div>
  );
}

function UsageDemo({ demo }) {
  return (
    <div style={{
      background: demo.bg,
      border: `1px solid ${demo.border}`,
      borderRadius: "5px",
      padding: "12px 16px",
      marginBottom: "8px",
    }}>
      <div style={{
        fontSize: "10px",
        fontFamily: "monospace",
        color: "#9BA3AF",
        letterSpacing: "0.1em",
        textTransform: "uppercase",
        marginBottom: "10px",
      }}>
        {demo.label}
      </div>
      <div style={{ display: "flex", gap: "24px", flexWrap: "wrap" }}>
        {demo.elements.map((el, i) => (
          <div key={i}>
            <div style={{
              fontSize: "10px",
              color: "#6B7280",
              marginBottom: "3px",
              fontFamily: "monospace",
            }}>{el.label}</div>
            <div style={{
              fontSize: el.mono ? "13px" : "14px",
              fontFamily: el.mono ? "monospace" : "Georgia, serif",
              color: el.color,
              fontWeight: el.mono ? "normal" : "500",
            }}>
              {el.label}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function DivisionBellPalette() {
  const [mode, setMode] = useState("dark");

  return (
    <div style={{
      background: mode === "dark" ? "#0D1117" : "#F4F4F5",
      minHeight: "100vh",
      fontFamily: "Georgia, 'Times New Roman', serif",
      padding: "0",
      transition: "background 0.3s",
    }}>
      {/* Header */}
      <div style={{
        background: mode === "dark" ? "#161D27" : "#1B3A5C",
        borderBottom: mode === "dark" ? "1px solid #243447" : "1px solid #14304d",
        padding: "24px 32px",
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        flexWrap: "wrap",
        gap: "16px",
      }}>
        <div>
          <div style={{
            fontFamily: "monospace",
            fontSize: "10px",
            color: mode === "dark" ? "#5B8DD9" : "#9BBFEF",
            letterSpacing: "0.25em",
            textTransform: "uppercase",
            marginBottom: "6px",
          }}>
            Division Bell — Colour System
          </div>
          <h1 style={{
            fontSize: "22px",
            fontWeight: "normal",
            color: "#fff",
            margin: 0,
            letterSpacing: "-0.01em",
          }}>
            {mode === "dark" ? "Dark Mode Palette" : "Light Mode Palette"}
          </h1>
        </div>
        {/* Toggle */}
        <div style={{ display: "flex", gap: "8px" }}>
          {["light", "dark"].map(m => (
            <button
              key={m}
              onClick={() => setMode(m)}
              style={{
                padding: "8px 16px",
                borderRadius: "4px",
                border: "none",
                cursor: "pointer",
                fontFamily: "monospace",
                fontSize: "12px",
                letterSpacing: "0.1em",
                textTransform: "uppercase",
                background: mode === m ? (m === "dark" ? "#5B8DD9" : "#fff") : "transparent",
                color: mode === m ? (m === "dark" ? "#0D1117" : "#1B3A5C") : "rgba(255,255,255,0.5)",
                border: mode === m ? "none" : "1px solid rgba(255,255,255,0.2)",
                transition: "all 0.2s",
              }}
            >
              {m}
            </button>
          ))}
        </div>
      </div>

      <div style={{ padding: "32px" }}>

        {mode === "dark" ? (
          <>
            {/* Dark palette */}
            <div style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
              gap: "12px",
              marginBottom: "40px",
            }}>
              {darkPalette.map((color, i) => (
                <Swatch key={i} color={color} mode="dark" />
              ))}
            </div>

            {/* Usage in context */}
            <div style={{
              marginBottom: "12px",
              fontFamily: "monospace",
              fontSize: "11px",
              color: "#5B8DD9",
              letterSpacing: "0.15em",
              textTransform: "uppercase",
            }}>
              Colours in context — vote table rows
            </div>
            {darkInUse.map((demo, i) => (
              <UsageDemo key={i} demo={demo} />
            ))}

            {/* Design notes */}
            <div style={{
              marginTop: "32px",
              padding: "20px 24px",
              background: "#161D27",
              border: "1px solid #243447",
              borderLeft: "3px solid #5B8DD9",
              borderRadius: "0 5px 5px 0",
            }}>
              <div style={{
                fontFamily: "monospace",
                fontSize: "10px",
                color: "#5B8DD9",
                letterSpacing: "0.15em",
                textTransform: "uppercase",
                marginBottom: "12px",
              }}>
                Dark Mode Design Notes
              </div>
              {[
                ["Background is not pure black", "Midnight Ink (#0D1117) has a blue undertone that keeps it in family with the parliamentary navy without feeling cold or harsh. Pure black (#000) makes the navy accents look purple."],
                ["Three surface levels", "Background → Surface 1 → Surface 2 creates depth for cards, modals, and hover states without introducing a fourth colour. Each step is subtle — if you can see a strong difference, it's too much contrast."],
                ["Text is warm, not white", "Parchment (#E8E4DC) carries the warmth of the light-mode off-white into dark mode. Pure white text on dark backgrounds causes eye strain on long reading sessions — important for a data-heavy site."],
                ["Accents are slightly brightened", "Both the red and green are ~10% brighter in dark mode than light mode. Dark backgrounds absorb colour; the light-mode values would look muddy without this adjustment."],
                ["Links use Clarity Blue, not Primary Navy", "The dark-mode navy (#2A5298) is used for primary buttons only. Links use Clarity Blue (#5B8DD9), which has enough luminance contrast against all three surface levels to pass WCAG AA without a heavy underline."],
                ["The Data Mono colour (Code Slate)", "A cool blue-grey (#7BAFD4) for monospace data — bill IDs, division numbers, vote tallies. Distinct from both the link colour and the body text, which helps users scan data tables quickly."],
              ].map(([title, body], i) => (
                <div key={i} style={{ marginBottom: "12px" }}>
                  <div style={{ fontSize: "13px", color: "#E8E4DC", fontFamily: "monospace", marginBottom: "3px" }}>
                    {title}
                  </div>
                  <div style={{ fontSize: "12px", color: "#9BA3AF", lineHeight: 1.6, fontFamily: "monospace" }}>
                    {body}
                  </div>
                </div>
              ))}
            </div>
          </>
        ) : (
          <>
            {/* Light palette */}
            <div style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
              gap: "12px",
              marginBottom: "32px",
            }}>
              {lightPalette.map((color, i) => (
                <Swatch key={i} color={color} mode="light" />
              ))}
            </div>
            <div style={{
              padding: "16px 20px",
              background: "#fff",
              border: "1px solid #e5e7eb",
              borderLeft: "3px solid #4A6FA5",
              borderRadius: "0 5px 5px 0",
              fontSize: "13px",
              color: "#6B7280",
              fontFamily: "monospace",
              lineHeight: 1.6,
            }}>
              Switch to Dark Mode to see the full dark palette and usage notes.
            </div>
          </>
        )}

        {/* CSS variables export */}
        <div style={{
          marginTop: "32px",
          background: mode === "dark" ? "#161D27" : "#fff",
          border: mode === "dark" ? "1px solid #243447" : "1px solid #e5e7eb",
          borderRadius: "5px",
          overflow: "hidden",
        }}>
          <div style={{
            padding: "10px 16px",
            background: mode === "dark" ? "#1E2A3A" : "#F4F4F5",
            borderBottom: mode === "dark" ? "1px solid #243447" : "1px solid #e5e7eb",
            fontFamily: "monospace",
            fontSize: "11px",
            color: mode === "dark" ? "#5B8DD9" : "#4A6FA5",
            letterSpacing: "0.1em",
          }}>
            CSS VARIABLES — {mode === "dark" ? "DARK MODE" : "LIGHT MODE"}
          </div>
          <pre style={{
            margin: 0,
            padding: "16px",
            fontSize: "11.5px",
            lineHeight: 1.75,
            color: mode === "dark" ? "#9BA3AF" : "#6B7280",
            fontFamily: "monospace",
            overflowX: "auto",
          }}>
{mode === "dark" ? `[data-theme="dark"] {
  --color-bg:               #0D1117;  /* Midnight Ink */
  --color-surface-1:        #161D27;  /* Deep Slate */
  --color-surface-2:        #1E2A3A;  /* Raised Slate */
  --color-border:           #243447;  /* Navy Divide */
  --color-primary:          #2A5298;  /* Muted Navy */
  --color-primary-bright:   #5B8DD9;  /* Clarity Blue */
  --color-text-primary:     #E8E4DC;  /* Parchment */
  --color-text-secondary:   #9BA3AF;  /* Warm Grey */
  --color-text-muted:       #6B7280;  /* Faded Slate */
  --color-alert:            #E03E55;  /* Tempered Red */
  --color-alert-surface:    #2A1018;  /* Red Tint */
  --color-positive:         #3D9970;  /* Forest */
  --color-positive-surface: #0D2018;  /* Green Tint */
  --color-data-mono:        #7BAFD4;  /* Code Slate */
}` : `:root {
  --color-bg:               #F4F4F5;  /* Off White */
  --color-surface-1:        #FFFFFF;  /* White */
  --color-border:           #E5E7EB;  /* Light border */
  --color-primary:          #1B3A5C;  /* Parliamentary Navy */
  --color-primary-bright:   #4A6FA5;  /* Steel Blue */
  --color-text-primary:     #1A1A1A;  /* Near Black */
  --color-text-secondary:   #6B7280;  /* Slate Grey */
  --color-alert:            #C41E3A;  /* Parliamentary Red */
  --color-positive:         #2D6A4F;  /* Dark Green */
}`}
          </pre>
        </div>

      </div>
    </div>
  );
}
