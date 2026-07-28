(() => {
  if ("scrollRestoration" in history) history.scrollRestoration = "manual";
  const scrollKey = `wiibridge:scroll:${window.location.pathname}`;
  const rememberScroll = () => {
    try {
      window.sessionStorage.setItem(scrollKey, String(window.scrollY));
    } catch (_) {}
  };
  const restoreScroll = () => {
    let saved = null;
    try {
      saved = window.sessionStorage.getItem(scrollKey);
      window.sessionStorage.removeItem(scrollKey);
    } catch (_) {}
    if (saved === null) return;
    const top = Number(saved);
    if (!Number.isFinite(top)) return;
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => window.scrollTo(0, top));
    });
  };
  window.addEventListener("beforeunload", rememberScroll);
  document.addEventListener("submit", rememberScroll, true);
  document.addEventListener("click", (event) => {
    const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
    if (!link) return;
    try {
      if (new URL(link.href, window.location.href).pathname === window.location.pathname) {
        rememberScroll();
      }
    } catch (_) {}
  }, true);

  document.querySelectorAll("details[data-persist-details]").forEach((details) => {
    const stateKey = `wiibridge:details:${details.dataset.persistDetails}`;
    try {
      const saved = window.sessionStorage.getItem(stateKey);
      if (saved !== null) details.open = saved === "open";
    } catch (_) {}
    const summary = details.querySelector(":scope > summary");
    let summaryTop = null;
    if (summary) {
      summary.addEventListener("click", () => {
        summaryTop = summary.getBoundingClientRect().top;
      });
    }
    details.addEventListener("toggle", () => {
      try {
        window.sessionStorage.setItem(stateKey, details.open ? "open" : "closed");
      } catch (_) {}
      if (summaryTop === null || !summary) return;
      const previousTop = summaryTop;
      summaryTop = null;
      window.requestAnimationFrame(() => {
        window.scrollBy(0, summary.getBoundingClientRect().top - previousTop);
      });
    });
  });
  restoreScroll();

  const panel = document.getElementById("pi-live");
  const text = (id, value) => {
    const element = document.getElementById(id);
    if (element) element.textContent = value;
  };
  const yes = (value, enabled, disabled) => value ? enabled : disabled;
  const automaticSwitch = panel && panel.dataset.automaticSwitch === "true";
  const switchButtons = document.querySelectorAll("[data-profile-switch]");
  const setSwitchReady = (ready) => {
    if (!automaticSwitch) return;
    switchButtons.forEach((button) => {
      button.disabled = button.dataset.baseDisabled === "true" || !ready;
    });
    const warning = document.getElementById("pi-switch-warning");
    if (warning) warning.hidden = ready;
  };
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
      setSwitchReady(pi.state === "ready" && pi.board_compatible &&
        pi.provisioned && pi.wifi_provisioned &&
        pi.usb_controller && pi.usb_controller !== "none" &&
        pi.usb_state && pi.usb_state !== "unknown" &&
        pi.usb_state !== "unavailable");
    } catch (_) {
      dot.classList.add("offline");
      text("pi-connection", "Unavailable");
      text("pi-updated", "Retrying");
      setSwitchReady(false);
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
      if (value.progress.state !== "Building") {
        rememberScroll();
        window.location.reload();
      }
    } catch (_) {}
  }
  refreshPi();
  refreshBuild();
  window.setInterval(refreshPi, 10000);
  window.setInterval(refreshBuild, 3000);
})();
