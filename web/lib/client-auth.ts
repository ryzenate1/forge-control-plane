export function redirectOnUnauthorized(response: Response): Response {
  if (response.status === 401 && typeof window !== "undefined") {
    window.location.assign("/");
  }
  return response;
}
