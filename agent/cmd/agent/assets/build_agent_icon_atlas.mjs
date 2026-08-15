/**
 * Build the RemoteIt Agent icon atlas from the same Lucide vector source used
 * by the web panel. Rendering the paths at 1024 px keeps every Windows icon
 * smooth and gives Overview plus all internal screens one visual system.
 *
 * Run from the repository root after `pnpm --dir web install`:
 *   node agent/cmd/agent/assets/build_agent_icon_atlas.mjs
 */

import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { writeFile } from "node:fs/promises";

const require = createRequire(import.meta.url);
const sharp = require("sharp");

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..", "..", "..");
const lucideRoot = join(repoRoot, "web", "node_modules", "lucide-react", "dist", "esm", "icons");

const CELL = 256;
const RENDER = 1024;

const icons = [
  ["monitor", "monitor"],
  ["panel", "square-arrow-out-up-right"],
  ["log", "notebook-text"],
  ["folder", "folder"],
  ["settings", "settings"],
  ["info", "info"],
  ["bolt", "zap"],
  ["pencil", "pencil"],
  ["link", "link"],
  ["copy", "copy"],
  ["pulse", "activity"],
  ["service", "power"],
  ["shield", "shield-check"],
  ["clock", "clock-3"],
  ["list", "list"],
  ["arrow", "chevron-right"],
  ["check", "check"],
  ["circle-check", "circle-check"],
  ["dot", null],
  ["update", "refresh-cw"],
  ["cpu", "cpu"],
];

const colors = [
  ["green", "#05A368"],
  ["ink", "#121F37"],
  ["muted", "#60728D"],
  ["white", "#FFFFFF"],
  ["orange", "#D97706"],
  ["red", "#CC4141"],
];

const escapeAttribute = (value) => String(value)
  .replaceAll("&", "&amp;")
  .replaceAll('"', "&quot;")
  .replaceAll("<", "&lt;")
  .replaceAll(">", "&gt;");

function nodeMarkup([tag, attributes]) {
  const props = Object.entries(attributes)
    .filter(([name]) => name !== "key")
    .map(([name, value]) => `${name}="${escapeAttribute(value)}"`)
    .join(" ");
  return `<${tag}${props ? ` ${props}` : ""}/>`;
}

async function iconSvg(moduleName, color, logicalName) {
  if (logicalName === "dot") {
    return `<svg xmlns="http://www.w3.org/2000/svg" width="${RENDER}" height="${RENDER}" viewBox="0 0 24 24"><circle cx="12" cy="12" r="5.25" fill="${color}"/></svg>`;
  }

  const moduleUrl = pathToFileURL(join(lucideRoot, `${moduleName}.mjs`)).href;
  const { __iconNode } = await import(moduleUrl);
  // At the 20-44 px sizes used by the native Windows canvas, a 2.1 Lucide
  // stroke stays visually even at 100/125/150/200% DPI. The previous 1.8
  // stroke became too faint after the final Windows downscale.
  const strokeWidth = logicalName === "check" ? 2.35 : 2.1;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${RENDER}" height="${RENDER}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round">${__iconNode.map(nodeMarkup).join("")}</svg>`;
}

const atlas = sharp({
  create: {
    width: CELL * icons.length,
    height: CELL * colors.length,
    channels: 4,
    background: { r: 0, g: 0, b: 0, alpha: 0 },
  },
});

const composites = [];
for (let row = 0; row < colors.length; row += 1) {
  const [, color] = colors[row];
  for (let column = 0; column < icons.length; column += 1) {
    const [logicalName, moduleName] = icons[column];
    const svg = await iconSvg(moduleName, color, logicalName);
    const input = await sharp(Buffer.from(svg))
      .resize(CELL, CELL, { kernel: sharp.kernel.lanczos3 })
      .png({ compressionLevel: 9, adaptiveFiltering: true })
      .toBuffer();
    composites.push({ input, left: column * CELL, top: row * CELL });
  }
}

const output = join(here, "remoteit-agent-icons.png");
const rendered = await atlas
  .composite(composites)
  .png({ compressionLevel: 9, adaptiveFiltering: true })
  .toBuffer();
await writeFile(output, rendered);
console.log(`Wrote ${output} (${CELL * icons.length}x${CELL * colors.length})`);
