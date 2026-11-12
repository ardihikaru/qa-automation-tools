import { createHash, timingSafeEqual } from 'crypto';

const digest = (s: string) => createHash('sha256').update(s).digest();

/** Constant-time check of an `Authorization: Bearer <token>` header. */
export function checkBearer(header: string | undefined, token: string): boolean {
  if (!header || !token) return false;
  const m = /^Bearer\s+(.+)$/i.exec(header.trim());
  if (!m) return false;
  return timingSafeEqual(digest(m[1]), digest(token));
}
