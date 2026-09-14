import type { Metadata } from "next";
import "./globals.css";
import { NodeProvider } from "@/components/node-provider";
import { Shell } from "@/components/shell";

export const metadata: Metadata = { title: { default: "ZION Web", template: "%s · ZION" }, description: "Local-first client for the ZION Protocol alpha network" };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><a className="skip-link" href="#main-content">Skip to content</a><NodeProvider><Shell>{children}</Shell></NodeProvider></body></html>;
}
