import { ImageResponse } from "next/og";

export const size = { width: 32, height: 32 };
export const contentType = "image/png";

// Generated at build time. The wordmark distilled to a single character +
// the oxide-red accent dot — recognisable in a 32px favicon slot.
export default function Icon() {
  return new ImageResponse(
    (
      <div
        style={{
          background: "#0e0c0a",
          width: "100%",
          height: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: "#f1ece3",
          fontFamily: "serif",
          fontSize: 22,
          fontWeight: 600,
          letterSpacing: "-0.02em",
          lineHeight: 1,
        }}
      >
        <span>l</span>
        <span style={{ color: "#c44536", marginLeft: -1 }}>.</span>
      </div>
    ),
    size,
  );
}
