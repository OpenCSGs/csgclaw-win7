import assert from "node:assert/strict";
import test from "node:test";
import { DesktopUpdater } from "./updater";

test("Win7 distribution cannot switch channels or start an ordinary update installer", async () => {
  const published: string[] = [];
  let beforeInstallCalled = false;
  const updater = new DesktopUpdater(
    (status) => published.push(status.state),
    async () => {
      beforeInstallCalled = true;
    },
    "0.9.0",
  );
  const initial = updater.currentStatus();
  updater.startBackgroundChecks();
  await updater.setChannel("release");
  await updater.setChannel("beta");
  await updater.checkForUpdates();
  await assert.rejects(
    updater.installDownloadedUpdate(),
    /Automatic updates are disabled/,
  );
  updater.stopBackgroundChecks();
  assert.deepEqual(updater.currentStatus(), initial);
  assert.equal(beforeInstallCalled, false);
  assert.deepEqual(published, Array(4).fill("unsupported"));
});
