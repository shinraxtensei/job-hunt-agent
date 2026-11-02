import type { ReactNode } from "react";

// safeHref returns url only when it is an absolute http(s) URL, so a posting can't
// smuggle in javascript:, data: or any other scheme (AC4).
export function safeHref(url: string): string | null {
  try {
    const u = new URL(url);
    return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
}

// ExternalLink renders a link to an untrusted URL, or plain text when it isn't safe.
export function ExternalLink({ href, children }: { href: string; children: ReactNode }) {
  const safe = safeHref(href);
  if (!safe) return <span>{children}</span>;
  return (
    <a href={safe} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  );
}
