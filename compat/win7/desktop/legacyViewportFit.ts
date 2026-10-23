import type { BrowserWindow } from "electron";

// Win7 can clamp the native window below the upstream layout's minimum at high
// DPI. Fit the existing layout instead of leaving its send/actions off screen.
export function legacyViewportZoom(
  width: number,
  height: number,
  currentZoom: number,
): number {
  if (
    ![width, height, currentZoom].every(
      (value) => Number.isFinite(value) && value > 0,
    )
  )
    return 1;
  return Math.max(
    0.5,
    Math.min(1, (width * currentZoom) / 980, (height * currentZoom) / 680),
  );
}

export function installLegacyViewportFit(window: BrowserWindow): void {
  const webContents = window.webContents;
  let generation = 0;
  let closed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const fit = async () => {
    const request = ++generation;
    if (closed || window.isDestroyed() || webContents.isDestroyed()) return;
    try {
      const zoom = webContents.getZoomFactor();
      const viewport: { width: number; height: number } =
        await webContents.executeJavaScript(
          "({width:window.innerWidth,height:window.innerHeight})",
        );
      if (
        closed ||
        request !== generation ||
        window.isDestroyed() ||
        webContents.isDestroyed()
      )
        return;
      const next = legacyViewportZoom(viewport.width, viewport.height, zoom);
      if (Math.abs(next - zoom) > 0.002) webContents.setZoomFactor(next);
    } catch {
      // Navigation and shutdown can destroy a pending renderer context.
    }
  };
  const schedule = () => {
    generation += 1;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      void fit();
    }, 80);
  };
  window.on("resize", schedule);
  webContents.on("dom-ready", schedule);
  window.once("closed", () => {
    closed = true;
    generation += 1;
    if (timer) clearTimeout(timer);
    window.removeListener("resize", schedule);
    webContents.removeListener("dom-ready", schedule);
  });
}
