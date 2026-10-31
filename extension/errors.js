export function toError(payload, status) {
  const message = payload?.message || `bridge HTTP ${status}`;
  const error = new Error(message);
  error.code = payload?.code || "";
  error.details = payload?.details || null;
  error.status = status;
  return error;
}

export function errorMessage(error) {
  return error?.message || String(error);
}