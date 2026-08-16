import hbSubsetWasmUrl from '../../node_modules/harfbuzzjs/dist/harfbuzz-subset.wasm?url';

type HbSubsetExports = {
  memory: WebAssembly.Memory;
  malloc: (size: number) => number;
  free: (pointer: number) => void;
  hb_blob_create: (data: number, length: number, mode: number, userData: number, destroy: number) => number;
  hb_blob_destroy: (blob: number) => void;
  hb_blob_get_data: (blob: number, lengthPointer: number) => number;
  hb_blob_get_length: (blob: number) => number;
  hb_face_create: (blob: number, index: number) => number;
  hb_face_destroy: (face: number) => void;
  hb_face_reference_blob: (face: number) => number;
  hb_subset_input_create_or_fail: () => number;
  hb_subset_input_destroy: (input: number) => void;
  hb_subset_input_unicode_set: (input: number) => number;
  hb_subset_input_set_flags: (input: number, flags: number) => void;
  hb_subset_input_pin_all_axes_to_default: (input: number) => number;
  hb_set_add: (set: number, codepoint: number) => void;
  hb_subset_or_fail: (face: number, input: number) => number;
};

const HB_MEMORY_MODE_WRITABLE = 2;
const HB_SUBSET_FLAGS_NO_HINTING = 1;

let hbPromise: Promise<HbSubsetExports> | undefined;

export async function subsetReportFont(font: Uint8Array, text: string): Promise<Uint8Array> {
  const hb = await loadHbSubset();
  const heap = () => new Uint8Array(hb.memory.buffer);
  const fontPointer = hb.malloc(font.byteLength);
  let blob = 0;
  let face = 0;
  let input = 0;
  let subset = 0;
  let resultBlob = 0;
  try {
    heap().set(font, fontPointer);
    blob = hb.hb_blob_create(fontPointer, font.byteLength, HB_MEMORY_MODE_WRITABLE, 0, 0);
    face = hb.hb_face_create(blob, 0);
    input = hb.hb_subset_input_create_or_fail();
    if (input === 0) throw new Error('Unable to create a font subset');
    hb.hb_subset_input_set_flags(input, HB_SUBSET_FLAGS_NO_HINTING);
    hb.hb_subset_input_pin_all_axes_to_default(input);
    const unicodes = hb.hb_subset_input_unicode_set(input);
    for (const character of text) {
      const codepoint = character.codePointAt(0);
      if (codepoint) hb.hb_set_add(unicodes, codepoint);
    }
    subset = hb.hb_subset_or_fail(face, input);
    if (subset === 0) throw new Error('Unable to subset the report font');
    resultBlob = hb.hb_face_reference_blob(subset);
    const length = hb.hb_blob_get_length(resultBlob);
    const offset = hb.hb_blob_get_data(resultBlob, 0);
    if (length === 0 || offset === 0) throw new Error('Font subset was empty');
    const bytes = new Uint8Array(length);
    bytes.set(heap().subarray(offset, offset + length));
    return bytes;
  } finally {
    if (resultBlob) hb.hb_blob_destroy(resultBlob);
    if (subset) hb.hb_face_destroy(subset);
    if (input) hb.hb_subset_input_destroy(input);
    if (face) hb.hb_face_destroy(face);
    if (blob) hb.hb_blob_destroy(blob);
    hb.free(fontPointer);
  }
}

export function releaseSubsetRuntime(): void {
  hbPromise = undefined;
}

async function loadHbSubset(): Promise<HbSubsetExports> {
  if (!hbPromise) {
    hbPromise = instantiateHbSubset().catch((error) => {
      hbPromise = undefined;
      throw error;
    });
  }
  return hbPromise;
}

async function instantiateHbSubset(): Promise<HbSubsetExports> {
  const source = await loadWasmBytes();
  const { instance } = await WebAssembly.instantiate(source);
  return instance.exports as unknown as HbSubsetExports;
}

async function loadWasmBytes(): Promise<ArrayBuffer> {
  try {
    const response = await fetch(hbSubsetWasmUrl);
    if (response.ok) return response.arrayBuffer();
  } catch {
    /* Tests load the wasm from disk. */
  }
  const { readFileSync } = await import('node:fs');
  const { resolve } = await import('node:path');
  const bytes = readFileSync(resolve(process.cwd(), 'node_modules/harfbuzzjs/dist/harfbuzz-subset.wasm'));
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}
