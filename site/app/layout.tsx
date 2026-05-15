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
  title: "logmind — branch-aware AI decision logging",
  description:
    "An opinionated decision log for AI-assisted development. One CLI command per architectural choice. Branch-aware. AGENTS.md-canonical.",
  metadataBase: new URL("https://logmind.dev"),
  openGraph: {
    title: "logmind",
    description: "Branch-aware AI decision logging for development projects.",
    url: "https://logmind.dev",
    siteName: "logmind",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "logmind",
    description: "Branch-aware AI decision logging for development projects.",
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
