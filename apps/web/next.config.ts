import type { NextConfig } from "next";

const apiOrigin = process.env.NEXT_PUBLIC_ZION_API_URL ?? "http://127.0.0.1:42001";
let connectOrigin = "http://127.0.0.1:42001";
try { connectOrigin = new URL(apiOrigin).origin; } catch { /* runtime validation reports malformed configuration */ }

const csp = [
  "default-src 'self'", "base-uri 'self'", "form-action 'self'", "frame-ancestors 'none'",
  "img-src 'self' data:", "object-src 'none'", `script-src 'self' 'unsafe-inline'${process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : ""}`, "style-src 'self' 'unsafe-inline'",
  `connect-src 'self' ${connectOrigin}`,
].join("; ");

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    return [{ source: "/(.*)", headers: [
      { key: "Content-Security-Policy", value: csp },
      { key: "Referrer-Policy", value: "no-referrer" },
      { key: "X-Content-Type-Options", value: "nosniff" },
      { key: "X-Frame-Options", value: "DENY" },
      { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=()" },
    ] }];
  },
};

export default nextConfig;
