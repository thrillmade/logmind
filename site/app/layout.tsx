import type { Metadata } from "next";
import { Fraunces, Geist_Mono } from "next/font/google";
import "./globals.css";

// Fraunces: an expressive variable serif with optical sizing, SOFT and WONK
// axes — gives the page real character without bespoke font hosting.
const fraunces = Fraunces({
  variable: "--font-fraunces",
  subsets: ["latin"],
  axes: ["opsz", "SOFT", "WONK"],
  display: "swap",
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
  display: "swap",
});

export const metadata: Metadata = {
  title: "logmind — infinite context for every agent you ever hire",
  description:
    "Capture AI decisions while you work. Every new agent gets the full why-behind-the-code in one read. AGENTS.md canonical, link integrity on every PR, project tree always current.",
  metadataBase: new URL("https://logmind.dev"),
  openGraph: {
    title: "logmind",
    description: "Infinite context for every agent. Captured while you work.",
    url: "https://logmind.dev",
    siteName: "logmind",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "logmind",
    description: "Infinite context for every agent. Captured while you work.",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${fraunces.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col relative">{children}</body>
    </html>
  );
}
