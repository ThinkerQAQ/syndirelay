// Read-only job metadata determines which browser-backed transports need
// their existing extension worker. Never dispatch or replay a publishing job.
const ACTIVE_STATES = new Set(["running", "queued", "waiting"]);

export function activeBrowserJobPumps(jobs) {
  const needed = { toutiao: false, medium: false };
  if (!Array.isArray(jobs)) return needed;
  for (const job of jobs) {
    if (!job || !ACTIVE_STATES.has(job.state) || !Array.isArray(job.platforms)) continue;
    if (job.platforms.includes("toutiao")) needed.toutiao = true;
    if (job.platforms.includes("medium")) needed.medium = true;
    if (needed.toutiao && needed.medium) break;
  }
  return needed;
}
