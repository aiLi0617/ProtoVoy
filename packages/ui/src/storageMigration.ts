export function readStorage(primary: string): string | null {
  try {
    return localStorage.getItem(primary);
  } catch {
    return null;
  }
}
