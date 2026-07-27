(() => {
  const panel = document.getElementById("pi-live");
  const text = (id, value) => {
    const element = document.getElementById(id);
    if (element) element.textContent = value;
  };
  const yes = (value, enabled, disabled) => value ? enabled : disabled;
  async function refreshPi() {
    if (!panel) return;
    const dot = document.getElementById("pi-dot");
    try {
      const response = await fetch(panel.dataset.statusUrl, {
        headers: {"Accept": "application/json"}, cache: "no-store"
      });
      if (!response.ok) throw new Error("unavailable");
      const value = await response.json();
      const pi = value.pi;
      dot.classList.remove("offline");
      text("pi-connection", "Connected");
      text("pi-state", pi.state || "unknown");
      text("pi-export", pi.export_mode || "not selected");
      text("pi-storage", yes(pi.nbd_connected, "NBD connected", "NBD disconnected") +
        " · " + yes(pi.usb_attached, "USB attached", "USB detached"));
      text("pi-usb", (pi.usb_controller || "none") + " · " + (pi.usb_state || "unknown"));
      text("pi-board", pi.detected_board || "unknown");
      text("pi-addresses", (pi.addresses || []).join(", ") || "none");
      text("pi-provision", yes(pi.provisioned, "Ready", "Incomplete"));
      text("pi-attach", yes(pi.auto_attach, "Enabled", "Disabled"));
      text("pi-updated", "Updated " + new Date().toLocaleTimeString());
    } catch (_) {
      dot.classList.add("offline");
      text("pi-connection", "Unavailable");
      text("pi-updated", "Retrying");
    }
  }
  const build = document.querySelector("[data-build-url]");
  async function refreshBuild() {
    if (!build) return;
    try {
      const response = await fetch(build.dataset.buildUrl, {
        headers: {"Accept": "application/json"}, cache: "no-store"
      });
      if (!response.ok) return;
      const value = await response.json();
      text("gc-build-state", value.progress.state);
      text("gc-current", value.progress.current_title || "Finalizing");
      text("gc-count", `${value.progress.titles_processed} / ${value.progress.total_titles} titles · ${value.progress.files_mapped} files · ${value.progress.extent_count} extents · ${value.progress.metadata_bytes_generated} metadata bytes · ${value.progress.current_phase}`);
      const progress = document.getElementById("gc-progress");
      if (progress) {
        progress.max = value.progress.total_titles || 1;
        progress.value = value.progress.titles_processed;
      }
      if (value.progress.state !== "Building") window.location.reload();
    } catch (_) {}
  }
  refreshPi();
  refreshBuild();
  window.setInterval(refreshPi, 10000);
  window.setInterval(refreshBuild, 3000);
})();
