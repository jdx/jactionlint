// Permalink encoding of the playground. Existing links (docs/checks.md has many) look like
// https://jactionlint.jdx.dev/#<base64 of deflate(utf-8 source)>, so this must stay
// compatible with the original playground.
import { deflate, inflate } from "pako";

function bytesToBinary(bytes: Uint8Array): string {
  // String.fromCharCode(...bytes) overflows the call stack for large inputs
  let s = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    s += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return s;
}

/** Encode workflow source to the value used after `#` in permalinks. */
export function encodeSource(source: string): string {
  const compressed = deflate(new TextEncoder().encode(source));
  return btoa(bytesToBinary(compressed));
}

/** Decode the value after `#` in a permalink (without the leading `#`). */
export function decodeSource(b64: string): string {
  let s = b64;
  try {
    s = decodeURIComponent(b64);
  } catch {
    // not percent-encoded
  }
  const compressed = Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
  return new TextDecoder().decode(inflate(compressed));
}
