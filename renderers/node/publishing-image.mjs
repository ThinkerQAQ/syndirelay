import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import sharp from "sharp";

import { ensureJar, renderSvg } from "../../scripts/plantuml/runtime.mjs";

const MAX_DIMENSION = 4096;

function parseArgs(argv) {
  const values = {};
  for (let index = 0; index < argv.length; index += 1) {
    const name = argv[index];
    if (!name.startsWith("--")) throw new Error("Unexpected renderer argument: " + name);
    const value = argv[index + 1];
    if (!value || value.startsWith("--")) throw new Error(name + " requires a value");
    values[name.slice(2)] = value;
    index += 1;
  }
  for (const required of ["kind", "input", "output"]) {
    if (!values[required]) throw new Error("--" + required + " is required");
  }
  return values;
}

function run(command, args, { cwd = process.cwd() } = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
      shell: false,
      windowsHide: true,
    });
    const output = [];
    child.stdout.on("data", (chunk) => output.push(chunk));
    child.stderr.on("data", (chunk) => output.push(chunk));
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      const combined = Buffer.concat(output).toString("utf8").trim();
      if (signal) reject(new Error(command + " terminated by signal " + signal + ": " + combined));
      else if (code !== 0) reject(new Error(command + " exited with code " + code + ": " + combined));
      else resolve(combined);
    });
  });
}

// The CLI is packaged alongside this renderer; never invoke npx or download packages.
const mermaidCLI = path.resolve(import.meta.dirname, "../../node_modules/@mermaid-js/mermaid-cli/src/cli.js");
let selectedBrowser;
function existingBrowser() {
 if (selectedBrowser !== undefined) return selectedBrowser;
 const candidates = [];
 if (process.env.MERMAID_BROWSER_EXECUTABLE) candidates.push(process.env.MERMAID_BROWSER_EXECUTABLE);
 if (process.platform === "win32") {
  const roots = [process.env.PROGRAMFILES, process.env["PROGRAMFILES(X86)"], process.env.LOCALAPPDATA].filter(Boolean);
  for (const root of roots) {
   candidates.push(path.join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
                   path.join(root, "Google", "Chrome", "Application", "chrome.exe"));
  }
 } else if (process.platform === "darwin") {
  candidates.push("/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
                  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome");
 } else {
  candidates.push("/usr/bin/microsoft-edge", "/usr/bin/microsoft-edge-stable",
                  "/usr/bin/google-chrome", "/usr/bin/google-chrome-stable",
                  "/usr/bin/chromium", "/usr/bin/chromium-browser");
 }
 selectedBrowser = candidates.find((candidate) => existsSync(candidate)) ?? null;
 return selectedBrowser;
}
async function renderMermaid(sourceFile, outputFile, renderer, width, scale) {
 const resolvedWidth = Number(width), resolvedScale = Number(scale);
 if (!Number.isFinite(resolvedWidth) || resolvedWidth <= 0) throw new Error("--width must be positive");
 if (!Number.isFinite(resolvedScale) || resolvedScale <= 0) throw new Error("--scale must be positive");
 if (!existsSync(mermaidCLI)) throw new Error("Bundled Mermaid CLI is missing; reinstall SyndiRelay");
 const browser = existingBrowser();
 if (!browser) throw new Error("Mermaid rendering requires an installed Edge/Chrome browser; no download is attempted");
 const temporary = await mkdtemp(path.join(os.tmpdir(), "syndirelay-mermaid-"));
 try {
  const configFile = path.join(temporary, "mermaid-config.json");
  const browserConfig = path.join(temporary, "puppeteer-config.json");
  await writeFile(configFile, JSON.stringify({securityLevel: "strict", theme: "default"}));
  // Disabling Chromium's sandbox is allowed only for explicitly configured CI
  // environments; never weaken the default user runtime.
  const browserArgs = ["--no-first-run"];
  if (process.env.MERMAID_TEST_DISABLE_BROWSER_SANDBOX === "1") {
    browserArgs.push("--no-sandbox", "--disable-setuid-sandbox");
  }
  await writeFile(browserConfig, JSON.stringify({executablePath: browser, args: browserArgs}));
  await run(process.execPath, [mermaidCLI, "--input", sourceFile, "--output", outputFile,
     "--backgroundColor", "white", "--width", String(resolvedWidth),
     "--scale", String(resolvedScale), "--configFile", configFile,
     "--puppeteerConfigFile", browserConfig], {cwd: temporary});
 } finally {
  await rm(temporary, {recursive: true, force: true});
 }
}

async function renderPlantUML(source, outputFile) {
  await ensureJar();
  const svg = await renderSvg(source);
  await sharp(Buffer.from(svg)).png().toFile(outputFile);
}

// Render aligned text diagrams as faithful monospace PNGs.
// SVG <text> elements (unlike foreignObject) survive Sharp rasterization.
function escapeASCIIForSVG(s) {
  return s.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}
async function renderASCII(source, outputFile) {
  const lines = source.replaceAll("\r\n","\n").replaceAll("\r","\n").replace(/\n$/, "").split("\n");
  if (lines.length > 130 || source.length > 24000) throw new Error("Text diagram exceeds size limit");
  const chars = Math.max(...lines.map(line => [...line].length));
  const fontSize = 19, xAdvance = 11.5, lineHeight = 30;
  const width = Math.ceil(chars * xAdvance + 70), height = Math.ceil(lines.length*lineHeight+55);
  if (width>4096 || height>4096) throw new Error("Text diagram exceeds raster limits");
  const rows = lines.map((line,i) => '<text x="36" y="'+(46+i*lineHeight)+'" xml:space="preserve">'+escapeASCIIForSVG(line)+'</text>').join("");
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="'+width+'" height="'+height+'">'
    +'<rect width="100%" height="100%" fill="#f8f8f8"/>'
    +'<g font-family="DejaVu Sans Mono, Consolas, monospace" font-size="'+fontSize+'" fill="#252525">'+rows+'</g></svg>';
  await sharp(Buffer.from(svg)).png().toFile(outputFile);
}
// Medium has no editable native table paragraph. Rasterize Markdown tables
// with native SVG text (no foreignObject), retaining the original row/column
// structure and readability. Escape all untrusted content as XML text.
function splitMarkdownTableRow(row) {
  let text=row.trim().replace(/^\|/, "").replace(/\|$/, "");
  const cells=[];let current="",ticks=0,escaped=false;
  for(let i=0;i<text.length;i++){
    const ch=text[i];
    if(escaped){current+=ch;escaped=false;continue;}
    if(ch==="\\"){escaped=true;continue;}
    if(ch==="`"){
      let run=1;while(text[i+run]==="`")run++;
      current+="`".repeat(run);i+=run-1;
      ticks=ticks===run?0:(ticks===0?run:ticks);continue;
    }
    if(ch==="|" && ticks===0){cells.push(current.trim());current="";continue;}
    current+=ch;
  }
  cells.push(current.trim());return cells;
}
function plainTableCell(value){
  return value.replace(/\*\*([^*]+)\*\*/g,"$1").replace(/__([^_]+)__/g,"$1")
    .replace(/`+([^`]+)`+/g,"$1").replace(/\[([^\]]+)\]\([^)]+\)/g,"$1").replace(/\*([^*]+)\*/g,"$1");
}
function weightedTextLength(text){
  return [...text].reduce((sum,ch)=>sum+(/[^\x00-\x7F]/u.test(ch)?1.7:/[MWmw@%]/u.test(ch)?1.2:/[iltI1.,:;]/u.test(ch)?0.46:0.95),0);
}
function wrapTableCell(raw,maxWidth,fontSize){
  const text=plainTableCell(raw).replace(/\s+/g," ").trim();
  const limit=maxWidth/(fontSize*0.59);
  const words=text.split(" ");const lines=[];let line="";
  for(const word of words){
    if(!word)continue;
    if(line && weightedTextLength(line+" "+word)<=limit){line+=" "+word;continue;}
    if(line){lines.push(line);line="";}
    if(weightedTextLength(word)<=limit){line=word;continue;}
    for(const c of word){
      if(line && weightedTextLength(line+c)>limit){lines.push(line);line="";}
      line+=c;
    }
  }
  if(line)lines.push(line);
  return lines.length?lines:[""];
}
async function renderTable(source,outputFile){
  const rows=source.replaceAll("\r\n","\n").trim().split("\n").map(splitMarkdownTableRow);
  if(rows.length<3 || rows.length>64)throw Error("Markdown table has invalid row count");
  const headers=rows[0],colCount=headers.length;
  if(colCount<2 || colCount>8 || rows.some(row=>row.length!==colCount))throw Error("Markdown table column mismatch");
  const values=[headers,...rows.slice(2)];
  const fontSize=22,lineHeight=32,padding=18,outer=22,border=1;
  const width=colCount===2?1080:Math.min(1800,Math.max(1080,colCount*275));
  const innerWidth=width-2*outer;
  const colWidths=colCount===2?[Math.round(innerWidth*.23),Math.round(innerWidth*.77)]:Array(colCount).fill(innerWidth/colCount);
  const wrapped=values.map(row=>row.map((cell,i)=>wrapTableCell(cell,colWidths[i]-2*padding,fontSize)));
  const rowHeights=wrapped.map(row=>Math.max(54,Math.max(...row.map(cell=>cell.length))*lineHeight+padding*2));
  const height=Math.ceil(rowHeights.reduce((a,b)=>a+b,0)+2*outer);
  if(width>4096||height>4096)throw Error("Rendered table exceeds size limits");
  const parts=['<svg xmlns="http://www.w3.org/2000/svg" width="'+width+'" height="'+height+'">',
    '<rect width="100%" height="100%" fill="#ffffff"/>'];
  let y=outer;
  for(let r=0;r<wrapped.length;r++){
    let x=outer;
    const rowHeight=rowHeights[r];
    for(let c=0;c<colCount;c++){
      const cw=colWidths[c];
      const fill=r===0?"#eef2f6":r%2===0?"#f8fafc":"#ffffff";
      parts.push(`<rect x="${x}" y="${y}" width="${cw}" height="${rowHeight}" fill="${fill}" stroke="#cbd5e1" stroke-width="${border}"/>`);
      const bold=r===0||c===0;
      for(let k=0;k<wrapped[r][c].length;k++){
        const t=wrapped[r][c][k];
        parts.push(`<text x="${x+padding}" y="${y+padding+fontSize+k*lineHeight}" font-family="DejaVu Sans, Arial, sans-serif" font-weight="${bold?600:400}" font-size="${fontSize}" fill="#242a31">${escapeASCIIForSVG(t)}</text>`);
      }
      x+=cw;
    }
    y+=rowHeight;
  }
  parts.push("</svg>");
  await sharp(Buffer.from(parts.join(""))).png().toFile(outputFile);
}

async function constrain(outputFile) {
  const input = await readFile(outputFile);
  const metadata = await sharp(input).metadata();
  const width = Number(metadata.width || 0);
  const height = Number(metadata.height || 0);
  if (!width || !height || (width <= MAX_DIMENSION && height <= MAX_DIMENSION)) return;
  const payload = await sharp(input)
    .resize({
      width: MAX_DIMENSION,
      height: MAX_DIMENSION,
      fit: "inside",
      withoutEnlargement: true,
    })
    .png()
    .toBuffer();
  await writeFile(outputFile, payload);
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const input = path.resolve(args.input);
  const output = path.resolve(args.output);
  await mkdir(path.dirname(output), { recursive: true });
  const source = await readFile(input, "utf8");

  if (args.kind === "mermaid") {
    await renderMermaid(input, output, args.renderer, args.width, args.scale);
  } else if (args.kind === "plantuml") {
    await renderPlantUML(source, output);
  } else if (args.kind === "ascii") {
    await renderASCII(source, output);
  } else if (args.kind === "table") {
    await renderTable(source, output);
  } else {
    throw new Error("Unsupported publishing asset kind: " + args.kind);
  }
  await constrain(output);
  console.log(JSON.stringify({ operation: "publishing-renderer", kind: args.kind, output }));
}

main().catch((error) => {
  console.error(JSON.stringify({
    operation: "publishing-renderer",
    status: "failed",
    error: error instanceof Error ? error.message : String(error),
  }));
  process.exitCode = 1;
});
