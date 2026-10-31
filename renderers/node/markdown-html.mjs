import { micromark } from "micromark";

function splitTableRow(line) {
  let text = String(line).trim();
  if (text.startsWith("|")) text = text.slice(1);
  if (text.endsWith("|")) text = text.slice(0, -1);
  const cells = [];
  let current = "";
  let escaped = false;
  let codeTicks = 0;
  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (escaped) { current += char; escaped = false; continue; }
    if (char === "\\") { current += char; escaped = true; continue; }
    if (char === "`") {
      let run = 1;
      while (text[index + run] === "`") run += 1;
      current += "`".repeat(run);
      index += run - 1;
      codeTicks = codeTicks === run ? 0 : (codeTicks === 0 ? run : codeTicks);
      continue;
    }
    if (char === "|" && codeTicks === 0) { cells.push(current.trim()); current = ""; continue; }
    current += char;
  }
  cells.push(current.trim());
  return cells;
}
function isTableDelimiter(line) {
  const cells = splitTableRow(line);
  return cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/u.test(cell.trim()));
}
function inlineMarkdownToHtml(value) {
  const rendered = micromark(String(value), { allowDangerousHtml: true }).trim();
  const match = rendered.match(/^<p>([\s\S]*)<\/p>$/u);
  return match ? match[1] : rendered;
}
function renderPlatformHtml(markdown) {
  const lines = String(markdown).replaceAll("\r\n", "\n").split("\n");
  const output = [];
  let index = 0;
  let fence = "";
  while (index < lines.length) {
    const line = lines[index];
    const fenceMatch = line.match(/^\s*([`~]{3,})/u);
    if (fenceMatch) {
      const token = fenceMatch[1][0];
      if (!fence) fence = token;
      else if (fence === token) fence = "";
      output.push(line);
      index += 1;
      continue;
    }
    if (!fence && index + 1 < lines.length && line.includes("|") && isTableDelimiter(lines[index + 1])) {
      const headers = splitTableRow(line);
      const delimiter = splitTableRow(lines[index + 1]);
      if (headers.length === delimiter.length && headers.length > 0) {
        const rows = [];
        index += 2;
        while (index < lines.length && lines[index].trim() && lines[index].includes("|")) {
          const row = splitTableRow(lines[index]);
          if (row.length !== headers.length) break;
          rows.push(row);
          index += 1;
        }
        const head = headers.map((cell) => "<th>" + inlineMarkdownToHtml(cell) + "</th>").join("");
        const body = rows.map((row) => "<tr>" + row.map((cell) => "<td>" + inlineMarkdownToHtml(cell) + "</td>").join("") + "</tr>").join("");
        output.push("<table><thead><tr>" + head + "</tr></thead><tbody>" + body + "</tbody></table>");
        continue;
      }
    }
    output.push(line);
    index += 1;
  }
  return micromark(output.join("\n"), { allowDangerousHtml: true });
}
let input = "";
for await (const chunk of process.stdin) input += chunk;
process.stdout.write(renderPlatformHtml(input));
