/** Convert a GitHub blob or Gist URL to the URL of the raw content. */
export function getUrlToFetch(u: string): string {
  const url = new URL(u);

  // /owner/repo/blob/branch/path/to -> raw.githubusercontent.com/owner/repo/branch/path/to
  if (url.host === "github.com") {
    const s = url.pathname.split("/blob/");
    if (s.length === 2) {
      url.pathname = s.join("/");
      url.host = "raw.githubusercontent.com";
      return url.toString();
    }
  }

  if (url.host === "gist.github.com" && /\/[0-9a-f]+$/.test(url.pathname)) {
    url.host = "gist.githubusercontent.com";
    url.pathname += "/raw";
    return url.toString();
  }

  return u;
}

export async function getRemoteSource(url: string): Promise<string> {
  const res = await fetch(getUrlToFetch(url));
  if (!res.ok) {
    throw new Error(`Fetching ${url} failed with status ${res.status}: ${res.statusText}`);
  }
  return (await res.text()).trim();
}

const reUrl = /https?:\/\/\S+/;

export interface MessagePart {
  text: string;
  url?: string;
}

/** Split an error message into text and http(s) URL parts so URLs can be rendered as links. */
export function linkifyMessage(text: string): MessagePart[] {
  const parts: MessagePart[] = [];
  let rest = text;
  for (;;) {
    const m = reUrl.exec(rest);
    if (m === null) {
      if (rest.length > 0) parts.push({ text: rest });
      return parts;
    }
    if (m.index > 0) parts.push({ text: rest.slice(0, m.index) });
    parts.push({ text: m[0], url: m[0] });
    rest = rest.slice(m.index + m[0].length);
  }
}
