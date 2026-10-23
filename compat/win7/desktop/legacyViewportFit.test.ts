import assert from "node:assert/strict";
import test from "node:test";
import { legacyViewportZoom } from "./legacyViewportFit";

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
