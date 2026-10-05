import type { Handle } from "@sveltejs/kit/hooks";

// One response boundary covers GET, native/enhanced actions and errors, without
// conflicting setHeaders calls when an action is followed by a server load.
export const handle: Handle = async ({ event, resolve }) => {
  const response = await resolve(event);
  if (event.route.id === "/" || event.route.id?.startsWith("/orders/")) {
    response.headers.set("cache-control", "no-store");
    response.headers.set("referrer-policy", "no-referrer");
  }
  return response;
};
