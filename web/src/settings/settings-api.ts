let session = "";

export function setSession(token: string) {
  session = token;
}

export async function request<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      "X-Tokenhub-Session": session,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  let result;
  try {
    result = JSON.parse(text);
  } catch {
    throw Error(text || `HTTP ${response.status}`);
  }
  if (!response.ok) throw Error(result.error || text);
  return result;
}
