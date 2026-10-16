// api.js talks to the server of graft ui with the token of this tab.

const KEY = 'graft-ui-token';
let token = '';

// takeToken moves the token from the address into the tab's
// sessionStorage, so that the address bar no longer shows it and a reload
// still has it.
export function takeToken() {
  const m = location.hash.match(/token=([0-9a-f]+)/);
  if (m) {
    token = m[1];
    try {
      sessionStorage.setItem(KEY, token);
    } catch {
      // No storage: the token lives until the tab reloads.
    }
    history.replaceState(null, '', location.pathname);
  } else {
    try {
      token = sessionStorage.getItem(KEY) || '';
    } catch {
      token = '';
    }
  }
  return token;
}

// ApiError is an answer other than 2xx, with its status and JSON body.
export class ApiError extends Error {
  constructor(status, body) {
    super(body && body.error ? body.error : `graft ui answered ${status}`);
    this.status = status;
    this.body = body;
  }
}

// call sends a request and returns the JSON answer. It throws ApiError for
// an answer other than 2xx, and TypeError when the server is gone.
export async function call(method, path, body) {
  const headers = { Authorization: `Bearer ${token}` };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  const resp = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  let data = null;
  try {
    data = await resp.json();
  } catch {
    // Not JSON: the status says enough.
  }
  if (!resp.ok) throw new ApiError(resp.status, data);
  return data;
}
