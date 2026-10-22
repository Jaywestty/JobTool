import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "jobtool",
  description: "Matched tech jobs with explainable match percentages.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
