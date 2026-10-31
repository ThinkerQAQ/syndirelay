import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, rename } from "node:fs/promises";
import path from "node:path";
import { JAR, JAR_SHA256, ROOT, VERSION, log, normalize, validateSvg } from "./core.mjs";

function execute(command, args, { input, timeout = 60000, cwd = ROOT } = {}) {
  return new Promise((resolve, reject) => {
    const child = execFile(command, args, { cwd, timeout, maxBuffer: 8 * 1024 * 1024, encoding: "utf8", windowsHide: true }, (error, stdout) => {
      if (error) {
        const failure = new Error(`${command === "java" ? "PlantUML/Java" : path.basename(command)} failed (${error.killed ? "timeout" : error.code}). Check Java 17+ and Graphviz availability, or the diagram syntax.`);
        failure.code = error.code;
        reject(failure);
      } else resolve(stdout);
    });
    child.stdin.on("error", () => {}); // Early compiler exits may close stdin.
    child.stdin.end(input ?? "");
  });
}

export async function ensureJar() {
  const startedAt = Date.now();
  await mkdir(path.dirname(JAR), { recursive: true });
  let bytes;
  try { bytes = await readFile(JAR); } catch (error) { if (error.code !== "ENOENT") throw error; }
  if (!bytes) {
    log("plantuml-download", "started", { version: VERSION });
    const temporary = `${JAR}.${process.pid}.download`;
    const url = `https://github.com/plantuml/plantuml/releases/download/v${VERSION}/plantuml.jar`;
    await execute(process.platform === "win32" ? "curl.exe" : "curl", ["--fail", "--location", "--retry", "2", "--connect-timeout", "15", "--max-time", "120", "--output", temporary, url], { timeout: 180000 });
    bytes = await readFile(temporary);
    if (createHash("sha256").update(bytes).digest("hex") !== JAR_SHA256) throw new Error("PlantUML download checksum mismatch; downloaded file was NOT executed");
    await rename(temporary, JAR);
  }
  if (createHash("sha256").update(bytes).digest("hex") !== JAR_SHA256) throw new Error("Cached PlantUML checksum mismatch; file was NOT executed");
  log("plantuml-runtime", "verified", { version: VERSION, durationMs: Date.now() - startedAt });
}

export async function renderSvg(source) {
  const temporary = path.join(ROOT, ".astro", "plantuml", "tmp");
  await mkdir(temporary, { recursive: true });
  const svg = await execute(process.env.PLANTUML_JAVA ?? "java", [
    "-Xmx256m", "-Djava.awt.headless=true", "-Dfile.encoding=UTF-8",
    `-Djava.io.tmpdir=${temporary}`, "-DPLANTUML_SECURITY_PROFILE=SANDBOX",
    "-jar", JAR, "-tsvg", "-pipe", "-charset", "UTF-8", "-nometadata", "-failfast2", "-timeout", "30",
  ], { input: normalize(source) });
  return validateSvg(svg);
}
