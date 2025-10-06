export const basePath = '/';

export async function fetchUiConfig() {
  try {
    const res = await fetch('/api/ui-config');
    if (!res.ok) return {} as Record<string, unknown>;
    return (await res.json()) as Record<string, unknown>;
  } catch {
    return {} as Record<string, unknown>;
  }
}
