// Alpine components used by the panel. Loaded before Alpine itself.
document.addEventListener("alpine:init", function () {
  Alpine.data("themeToggle", function () {
    return {
      toggle: function () {
        var root = document.documentElement;
        var current =
          root.dataset.theme ||
          (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
        var next = current === "dark" ? "light" : "dark";
        root.dataset.theme = next;
        try {
          localStorage.setItem("tellus-theme", next);
        } catch (e) {}
      },
    };
  });

  // Put data-confirm="message" on a form to ask before it submits.
  Alpine.data("confirmSubmit", function () {
    return {
      ask: function (event) {
        if (!window.confirm(this.$el.dataset.confirm)) event.preventDefault();
      },
    };
  });
});
