// Runs before first paint so a saved theme does not flash the wrong colors.
(function () {
  try {
    var t = localStorage.getItem("tellus-theme");
    if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
  } catch (e) {}
})();
