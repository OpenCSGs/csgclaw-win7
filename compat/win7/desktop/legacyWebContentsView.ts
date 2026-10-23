import { BrowserView, type BrowserWindow, type Rectangle } from "electron";

// Electron 22 represents embedded top-level pages as BrowserViews. Keep the
// upstream manager's visibility contract: hidden pages are detached, not merely
// moved behind the authenticated application renderer.
export class LegacyWebContentsView {
  readonly nativeView: BrowserView;
  private window: BrowserWindow | null = null;
  private visible = false;
  private attached = false;
  private bounds: Rectangle = { x: 0, y: 0, width: 0, height: 0 };

  constructor(
    options: Electron.BrowserViewConstructorOptions,
    createView: (options: Electron.BrowserViewConstructorOptions) => BrowserView =
      (preferences) => new BrowserView(preferences),
  ) {
    this.nativeView = createView(options);
  }
  get webContents() {
    return this.nativeView.webContents;
  }
  attach(window: BrowserWindow): void {
    this.detach();
    this.window = window;
    this.updateAttachment();
  }
  detach(): void {
    if (this.attached && this.window && !this.window.isDestroyed())
      this.window.removeBrowserView(this.nativeView);
    this.attached = false;
    this.window = null;
  }
  setBounds(bounds: Rectangle): void {
    this.bounds = bounds;
    if (this.attached) this.nativeView.setBounds(bounds);
  }
  setVisible(visible: boolean): void {
    this.visible = visible;
    this.updateAttachment();
  }
  private updateAttachment(): void {
    if (!this.window || this.window.isDestroyed()) return;
    if (this.visible && !this.attached) {
      this.window.addBrowserView(this.nativeView);
      this.attached = true;
      this.nativeView.setBounds(this.bounds);
    } else if (!this.visible && this.attached) {
      this.window.removeBrowserView(this.nativeView);
      this.attached = false;
    }
  }
}
