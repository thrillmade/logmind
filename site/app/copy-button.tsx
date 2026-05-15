"use client";

import { useState } from "react";

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);

  const handle = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      // ignore — older browsers without clipboard access
    }
  };

  return (
    <button
      type="button"
      onClick={handle}
      aria-label={label ?? "Copy to clipboard"}
      className="text-muted hover:text-foreground transition-colors text-xs uppercase tracking-wider px-2 py-1 border border-rule rounded-sm"
    >
      {copied ? "copied" : "copy"}
    </button>
  );
}
