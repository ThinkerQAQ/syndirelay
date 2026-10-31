import { createHash } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const VERSION = "1.2026.7";
export const JAR_SHA256 = "33aa7ed0ca843e300690230d09268e1f526fdde7e86fecdfa39fb80412cafcde";
export const ROOT = fileURLToPath(new URL("../../", import.meta.url));
export const JAR = path.join(ROOT, ".astro", "tools", `plantuml-${VERSION}.jar`);
export const CACHE = path.join(ROOT, ".astro", "plantuml", "svg");
export const OUTPUT = path.join(ROOT, "public", "diagrams", "plantuml");
export const MANIFEST = path.join(ROOT, ".astro", "plantuml", "manifest.json");

export function log(operation, status, details = {}, severity = "info") {
  console.log(JSON.stringify({ timestamp: new Date().toISOString(), severity, operation, status, ...details }));
}

export function isPlantuml(node) {
  return node.type === "code" && /^(puml|plantuml)$/i.test(node.lang ?? "");
}

export function visitCode(tree, callback) {
  for (const [index, node] of (tree.children ?? []).entries()) {
    if (isPlantuml(node)) callback(node, tree, index);
    else if (node.children) visitCode(node, callback);
  }
}

export function normalize(source) {
  let text = source.replaceAll("\r\n", "\n").trim();
  if (!text) throw new Error("Empty PlantUML diagram");
  // The JVM sandbox is the primary boundary; reject external preprocessing explicitly too.
  if (/^\s*!\s*(?:include\w*|import|theme)\b/im.test(text) || /%(?:getenv|load\w*|filename|dirpath)\s*\(/i.test(text)) {
    throw new Error("External includes, themes and environment/file access are disabled for diagrams");
  }
  const starts = [...text.matchAll(/^\s*@start(\w+)\b/gim)];
  const ends = [...text.matchAll(/^\s*@end(\w+)\b/gim)];
  if (starts.length === 0 && ends.length === 0) text = `@startuml\n${text}\n@enduml`;
  else if (starts.length !== 1 || ends.length !== 1 || starts[0][1].toLowerCase() !== ends[0][1].toLowerCase() || starts[0].index > ends[0].index) {
    throw new Error("Each code block must contain exactly one matching @start… / @end… pair");
  }
  return text + "\n";
}

export function diagramKey(source) {
  return createHash("sha256").update(`plantuml:${VERSION}:sandbox:utf8:svg:v1\n${normalize(source)}`).digest("hex");
}

export function diagramUrl(key) {
  if (!/^[a-f0-9]{64}$/.test(key)) throw new Error("Invalid diagram key");
  return `/diagrams/plantuml/${key}.svg`;
}

export function validateSvg(svg) {
  if (!/<svg\b[^>]*[\s\S]*<\/svg>/.test(svg) || /Syntax Error|An error has occurr?ed|Error line \d/i.test(svg)) {
    throw new Error("PlantUML did not return a valid diagram");
  }
  if (/<(?:script|foreignObject|!DOCTYPE|!ENTITY)\b|\son\w+\s*=|(?:href|xlink:href)\s*=\s*["'](?!#)/i.test(svg)) {
    throw new Error("SVG contains active content or external references");
  }
  return svg;
}
