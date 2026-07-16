import { ImageResponse } from "next/og";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "logmind — infinite context for every agent you ever hire";

// Editorial OG card — warm-ink background, large serif wordmark with the
// oxide-red dot, mono tagline + URL. Single source of truth for the social
// preview on Twitter, LinkedIn, Slack, Discord, etc.
export default async function OG() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          background: "#0e0c0a",
          color: "#f1ece3",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "72px 96px",
          fontFamily: "serif",
          position: "relative",
        }}
      >
        {/* Top rule + URL */}
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            fontFamily: "monospace",
            fontSize: 18,
            color: "#8a8278",
            letterSpacing: "0.18em",
            textTransform: "uppercase",
          }}
        >
          <span>logmind.dev</span>
          <span>v2.0 ⁄ MIT</span>
        </div>

        {/* Main composition */}
        <div style={{ display: "flex", flexDirection: "column", gap: 28 }}>
          <div
            style={{
              fontSize: 184,
              fontWeight: 600,
              letterSpacing: "-0.04em",
              lineHeight: 0.92,
              display: "flex",
              alignItems: "baseline",
            }}
          >
            <span>logmind</span>
            <span style={{ color: "#c44536", marginLeft: -10 }}>.</span>
          </div>
          <div
            style={{
              fontSize: 44,
              fontStyle: "italic",
              color: "#f1ece3",
              maxWidth: 1000,
              lineHeight: 1.15,
              letterSpacing: "-0.01em",
              display: "flex",
              flexWrap: "wrap",
              alignItems: "baseline",
              gap: 0,
            }}
          >
            <span>Infinite context</span>
            <span style={{ color: "#c44536", fontStyle: "normal" }}>.&nbsp;</span>
            <span style={{ color: "#8a8278" }}>
              For every agent you ever hire.
            </span>
          </div>
        </div>

        {/* Bottom mono caption */}
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            fontFamily: "monospace",
            fontSize: 18,
            color: "#8a8278",
            letterSpacing: "0.08em",
          }}
        >
          <span>$ brew install logmind</span>
          <span style={{ color: "#c44536" }}>●</span>
          <span>$ curl logmind.dev/install.sh | sh</span>
        </div>
      </div>
    ),
    size,
  );
}
