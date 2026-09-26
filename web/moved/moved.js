// The "We've moved" page, served at the web game's old address. A browser
// keeps saves per address, so this page reads the player's saves from this
// address's local storage and opens the new address with them in the URL
// fragment (#import=k/n:id:data), which browsers never send to a server.
// The game there asks before replacing anything, imports once and clears
// the fragment. The format is described in internal/move/move.go, and
// internal/move/moved_test.go runs this file in Node against the game's
// decoder.
(function () {
  "use strict";

  // Keep these in step with internal/move (a test checks).
  const OLD_URL = "https://halpworld.github.io/halpwords/";
  const PLAY_URL = "https://play.halpwords.com/";
  const PREFIX = "halpwords/"; // the game's keys in local storage
  const MAX_PART = 900000; // payload characters per fragment

  // exportable reports whether a save file moves. The AI keys (ai.json)
  // stay behind, as a URL can end up in the browser's history.
  function exportable(name) {
    if (!name || name.length > 255 || name.startsWith("/") || name.endsWith("/") ||
        name.includes("//") || /[\u0000-\u001f\u007f-\u009f\\]/.test(name)) {
      return false;
    }
    if (name.split("/").some((p) => p === "." || p === "..")) return false;
    return !(name === "ai.json" || name === "crash.txt" || name.endsWith(".bad") || name.startsWith("move/"));
  }

  // collect returns the game's files in storage (local storage, or
  // anything with length, key and getItem), by name.
  function collect(storage) {
    const names = [];
    for (let i = 0; i < storage.length; i++) {
      const key = storage.key(i);
      if (key !== null && key.startsWith(PREFIX) && exportable(key.slice(PREFIX.length))) {
        names.push(key.slice(PREFIX.length));
      }
    }
    names.sort();
    const files = {};
    for (const n of names) files[n] = storage.getItem(PREFIX + n);
    return files;
  }

  function base64url(bytes) {
    let bin = "";
    for (let i = 0; i < bytes.length; i += 0x8000) {
      bin += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
    }
    return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  // encode packs files into a payload: "z" and raw DEFLATE, or "j" and
  // plain JSON where the browser can't compress.
  async function encode(files) {
    const sorted = {};
    for (const n of Object.keys(files).sort()) {
      if (exportable(n)) sorted[n] = files[n];
    }
    const json = new TextEncoder().encode(JSON.stringify({ v: 1, files: sorted }));
    let cs;
    try {
      cs = new CompressionStream("deflate-raw");
    } catch (e) {
      return "j" + base64url(json); // an older browser
    }
    const stream = new Blob([json]).stream().pipeThrough(cs);
    return "z" + base64url(new Uint8Array(await new Response(stream).arrayBuffer()));
  }

  // id is the first 16 hex digits of the payload's SHA-256.
  async function id(payload) {
    const sum = new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(payload)));
    return Array.from(sum.subarray(0, 8), (b) => b.toString(16).padStart(2, "0")).join("");
  }

  // fragments cuts a payload into URL fragments of at most size payload
  // characters each.
  async function fragments(payload, size) {
    const n = Math.max(1, Math.ceil(payload.length / size));
    const h = await id(payload);
    const out = [];
    for (let k = 1; k <= n; k++) {
      out.push("import=" + k + "/" + n + ":" + h + ":" + payload.slice((k - 1) * size, k * size));
    }
    return out;
  }

  // target is the address to open: the new one, with part k of the saves
  // when there are any.
  async function target(storage, k) {
    let files = {};
    try {
      files = collect(storage);
    } catch (e) {
      // no local storage: nothing to bring
    }
    if (Object.keys(files).length === 0) return PLAY_URL;
    const parts = await fragments(await encode(files), MAX_PART);
    return k >= 1 && k <= parts.length ? PLAY_URL + "#" + parts[k - 1] : PLAY_URL;
  }

  // main runs on the page. It also works pasted into the browser's console
  // on the old address, to try the hand-over before the page goes live.
  async function main() {
    const link = document.getElementById("go");
    const status = document.getElementById("status");
    const m = /^#send=(\d+)$/.exec(location.hash);
    const k = m ? Number(m[1]) : 1;
    let url = PLAY_URL;
    try {
      url = await target(window.localStorage, k);
    } catch (e) {
      if (status) status.textContent = "Your progress couldn't be packed up. You can still play at the new address.";
    }
    if (link) link.href = url;
    location.replace(url);
  }

  if (typeof module === "object" && module.exports) {
    module.exports = { OLD_URL, PLAY_URL, PREFIX, MAX_PART, exportable, collect, encode, id, fragments, target };
  } else {
    main();
  }
})();
