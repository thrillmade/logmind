import { ImageResponse } from "next/og";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

export default function AppleIcon() {
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
          fontSize: 128,
          fontWeight: 600,
          letterSpacing: "-0.03em",
          lineHeight: 1,
        }}
      >
        <span>l</span>
        <span style={{ color: "#c44536", marginLeft: -6 }}>.</span>
      </div>
    ),
    size,
  );
}
