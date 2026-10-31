import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import sharp from "sharp";

const script=new URL("./sanitize-image.mjs",import.meta.url).pathname;
test("real EXIF metadata is stripped and orientation applied before output",async()=>{
 const input=await sharp({create:{width:5,height:3,channels:3,background:"#bca"}})
  .withMetadata({orientation:6})
  .withExif({IFD0:{Make:"PRIVATE_CAMERA",Model:"PRIVATE_MODEL"}})
  .jpeg().toBuffer();
 const original=await sharp(input).metadata();
 assert.ok(original.exif,"fixture has embedded EXIF");
 const dir=await mkdtemp(join(tmpdir(),"blogctl-sanitize-test-"));
 try {
  for(const format of ["webp","jpeg","png"]) {
   const dest=join(dir,"clean."+format);
   const run=spawnSync(process.execPath,[script,format,dest],{input,timeout:10000});
   assert.equal(run.status,0,run.stderr.toString());
   const result=JSON.parse(run.stdout.toString());
   const clean=await sharp(await readFile(dest)).metadata();
   assert.equal(result.privacyCleaned,true);
   assert.equal(clean.format,format);
   assert.equal(clean.width,3);
   assert.equal(clean.height,5);
   assert.equal(clean.exif,undefined);
   assert.equal(clean.xmp,undefined);
   assert.equal(clean.iptc,undefined);
   assert.equal(clean.icc,undefined);
   assert.equal(clean.orientation,undefined);
  }
 } finally {await rm(dir,{recursive:true,force:true})}
});
test("nonimage bytes are rejected, not published",async()=>{
 const dir=await mkdtemp(join(tmpdir(),"blogctl-invalid-image-"));
 try {
  const run=spawnSync(process.execPath,[script,"webp",join(dir,"out.webp")],{input:Buffer.from("<svg></svg>"),timeout:10000});
  assert.notEqual(run.status,0);
 } finally {await rm(dir,{recursive:true,force:true})}
});
