// Local image sanitizer. Raw pixels enter on stdin; only cleaned pixels leave.
import { writeFile } from "node:fs/promises";
import sharp from "sharp";

const format = process.argv[2], output = process.argv[3];
const mime = {webp:"image/webp", jpeg:"image/jpeg", png:"image/png"};
if (!Object.hasOwn(mime, format) || !output) throw Error("invalid sanitizer arguments");
const chunks=[]; let total=0;
for await (const chunk of process.stdin) {
  total+=chunk.length;
  if (total>20*1024*1024) throw Error("image exceeds 20 MiB");
  chunks.push(chunk);
}
if (!total) throw Error("empty image");
const original=Buffer.concat(chunks);
const opts={failOn:"error",limitInputPixels:40_000_000};
const before=await sharp(original,opts).metadata();
if (!["jpeg","png","webp"].includes(before.format)) throw Error("unsupported image format");
if (!before.width || !before.height || before.width*before.height>40_000_000) throw Error("invalid image dimensions");
let pipeline=sharp(original,opts).rotate();
switch (format) {
  case "webp": pipeline=pipeline.webp({quality:92,effort:4}); break;
  case "jpeg": pipeline=pipeline.jpeg({quality:92,progressive:false}); break;
  case "png": pipeline=pipeline.png({compressionLevel:9}); break;
}
const sanitized=await pipeline.toBuffer();
if (sanitized.length>24*1024*1024) throw Error("sanitized image exceeds 24 MiB");
// Re-read output to reject accidental metadata leakage.
const after=await sharp(sanitized,opts).metadata();
if (after.format!==format || !after.width || !after.height ||
    after.exif || after.xmp || after.iptc || after.icc || after.orientation) {
  throw Error("sanitized image failed privacy verification");
}
// Never call keepMetadata(), withMetadata(), or related metadata-retaining APIs.
await writeFile(output,sanitized,{mode:0o600});
process.stdout.write(JSON.stringify({format,contentType:mime[format],
  width:after.width,height:after.height,privacyCleaned:true})+"\n");
