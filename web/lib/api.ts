const apiBase = process.env.BACKEND_URL ?? "http://localhost:8080";

export type Health = {
  status: string;
  checks?: Record<string, string>;
};

export const BACKEND_BASE = apiBase;

export async function getHealth(): Promise<Health | null> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 3000);
  try {
    const res = await fetch(`${apiBase}/health`, {
      cache: "no-store",
      signal: controller.signal,
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as Health | null;
      return body ?? { status: `http_${res.status}` };
    }
    return (await res.json()) as Health;
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}
