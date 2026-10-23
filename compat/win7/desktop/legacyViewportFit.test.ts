import assert from "node:assert/strict";
import test from "node:test";
import { EventEmitter } from "node:events";
import { setImmediate } from "node:timers/promises";
import type { BrowserWindow } from "electron";
import {
  installLegacyViewportFit,
  legacyViewportZoom,
} from "./legacyViewportFit";

test("Win7 high-DPI work area retains the full composer and native titlebar layout", () => {
  const zoom = legacyViewportZoom(800, 560, 1);
  assert.ok(800 / zoom >= 980);
  assert.ok(560 / zoom >= 680);
  assert.ok(zoom < 1);
});
test("normal desktop sizes retain their original scale", () => {
  assert.equal(legacyViewportZoom(1440, 920, 1), 1);
});
test("fitting is stable when measured again through the scaled renderer", () => {
  const zoom = legacyViewportZoom(800, 560, 1);
  assert.equal(legacyViewportZoom(800 / zoom, 560 / zoom, zoom), zoom);
});
test("invalid and very small intermediate resize measurements are bounded", () => {
  assert.equal(legacyViewportZoom(0, 0, 1), 1);
  assert.equal(legacyViewportZoom(Number.NaN, 560, 1), 1);
  assert.equal(legacyViewportZoom(100, 100, 1), 0.5);
});

function viewportHarness() {
  let destroyed = false;
  const window = new EventEmitter();
  const contents = new EventEmitter();
  const zooms: number[] = [];
  let beginRead!: () => void;
  let completeRead!: (viewport: { width: number; height: number }) => void;
  const readStarted = new Promise<void>((resolve) => {
    beginRead = resolve;
  });
  const viewport = new Promise<{ width: number; height: number }>((resolve) => {
    completeRead = resolve;
  });
  Object.assign(contents, {
    isDestroyed: () => destroyed,
    getZoomFactor: () => 1,
    executeJavaScript: () => {
      beginRead();
      return viewport;
    },
    setZoomFactor: (zoom: number) => zooms.push(zoom),
  });
  Object.defineProperty(window, "webContents", {
    get() {
      if (destroyed) throw new Error("Object has been destroyed");
      return contents;
    },
  });
  Object.assign(window, { isDestroyed: () => destroyed });
  installLegacyViewportFit(window as unknown as BrowserWindow);
  return {
    window,
    contents,
    zooms,
    readStarted,
    completeRead,
    close() {
      destroyed = true;
      window.emit("closed");
    },
  };
}

test("closing a destroyed window detaches viewport listeners safely", () => {
  const h = viewportHarness();
  h.window.emit("resize");
  assert.doesNotThrow(() => h.close());
  assert.equal(h.window.listenerCount("resize"), 0);
  assert.equal(h.contents.listenerCount("dom-ready"), 0);
});

test(
  "late viewport measurements cannot resize a closed window",
  { timeout: 3000 },
  async () => {
    const h = viewportHarness();
    h.window.emit("resize");
    await h.readStarted;
    h.close();
    h.completeRead({ width: 800, height: 560 });
    await setImmediate();
    assert.deepEqual(h.zooms, []);
  },
);
