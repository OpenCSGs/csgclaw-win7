import assert from "node:assert/strict";
import test from "node:test";
import type { BrowserView, BrowserWindow, Rectangle } from "electron";
import { LegacyWebContentsView } from "./legacyWebContentsView";

function fixture() {
  const appliedBounds: Rectangle[] = [];
  const nativeView = {
    webContents: { identity: "same isolated page" },
    setBounds: (bounds: Rectangle) => appliedBounds.push(bounds),
  } as unknown as BrowserView;
  const view = new LegacyWebContentsView({}, () => nativeView);
  const windows = () => {
    const children = new Set<BrowserView>();
    let destroyed = false;
    return {
      children,
      destroy: () => { destroyed = true; },
      window: {
        isDestroyed: () => destroyed,
        addBrowserView: (child: BrowserView) => {
          assert.equal(destroyed, false);
          assert.equal(children.has(child), false, "a page cannot attach twice");
          children.add(child);
        },
        removeBrowserView: (child: BrowserView) => {
          assert.equal(destroyed, false);
          children.delete(child);
        },
      } as unknown as BrowserWindow,
    };
  };
  return { view, nativeView, appliedBounds, windows };
}

test("hidden browser tabs detach from the native window and release its input area", () => {
  const { view, nativeView, appliedBounds, windows } = fixture();
  const owner = windows();
  view.attach(owner.window);
  view.setBounds({ x: 100, y: 80, width: 500, height: 400 });
  assert.equal(owner.children.size, 0);
  view.setVisible(true);
  view.setVisible(true);
  assert.deepEqual([...owner.children], [nativeView]);
  view.setVisible(false);
  assert.equal(owner.children.size, 0);
  view.setBounds({ x: 200, y: 90, width: 420, height: 360 });
  view.setVisible(true);
  assert.deepEqual(appliedBounds.at(-1), { x: 200, y: 90, width: 420, height: 360 });
  assert.equal(view.webContents, nativeView.webContents);
});

test("moving a page to a new owner removes it from the previous window", () => {
  const { view, nativeView, windows } = fixture();
  const first = windows(), second = windows();
  view.attach(first.window);
  view.setVisible(true);
  view.attach(second.window);
  assert.equal(first.children.size, 0);
  assert.deepEqual([...second.children], [nativeView]);
  view.detach();
  assert.equal(second.children.size, 0);
});

test("owner shutdown does not call native methods on a destroyed window", () => {
  const { view, windows } = fixture();
  const owner = windows();
  view.attach(owner.window);
  view.setVisible(true);
  owner.destroy();
  assert.doesNotThrow(() => { view.setVisible(false); view.detach(); });
});
