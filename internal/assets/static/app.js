// Alpine components used by the panel. Loaded before Alpine itself.

// Date helpers shared by the datePicker component. All dates are handled as
// local-time y/m/d parts, since the panel only ever stores a calendar date
// (or a date and a wall-clock time), never an instant.
function isoDate(d) {
  var m = d.getMonth() + 1,
    day = d.getDate();
  return d.getFullYear() + "-" + (m < 10 ? "0" + m : m) + "-" + (day < 10 ? "0" + day : day);
}
function isoParts(s) {
  return { y: +s.slice(0, 4), m: +s.slice(5, 7) - 1, d: +s.slice(8, 10) };
}
function todayParts() {
  var d = new Date();
  return { y: d.getFullYear(), m: d.getMonth(), d: d.getDate() };
}
function weekdayLabels() {
  var fmt = new Intl.DateTimeFormat(undefined, { weekday: "short" });
  var labels = [];
  for (var i = 0; i < 7; i++) labels.push(fmt.format(new Date(2024, 0, 7 + i))); // 2024-01-07 is a Sunday
  return labels;
}

function initDashboardCharts() {
  var ink = getComputedStyle(document.documentElement).getPropertyValue("--ink").trim();
  document.querySelectorAll(".dashboard-chart").forEach(function (el) {
    el.replaceChildren(); // clear a previous draw, so a theme change can redraw cleanly
    var data = JSON.parse(el.dataset.series);
    new uPlot(
      {
        width: el.clientWidth,
        height: 60,
        series: [{}, { stroke: ink, width: 2, fill: ink + "22" }],
        axes: [{ show: false }, { show: false }],
        legend: { show: false },
        cursor: { show: false },
      },
      data,
      el
    );
  });
}
document.addEventListener("DOMContentLoaded", initDashboardCharts);

function initDonutCharts() {
  document.querySelectorAll(".donut-chart").forEach(function (el) {
    var existing = Chart.getChart(el);
    if (existing) existing.destroy(); // clear a previous draw, same reason initDashboardCharts clears uPlot's
    var segs = JSON.parse(el.dataset.segments);
    var style = getComputedStyle(document.documentElement);
    new Chart(el, {
      type: "doughnut",
      data: {
        labels: segs.map(function (s) { return s.label; }),
        datasets: [{
          data: segs.map(function (s) { return s.value; }),
          backgroundColor: segs.map(function (s) { return style.getPropertyValue(s.color).trim(); }),
          borderWidth: 0,
        }],
      },
      options: {
        cutout: "60%",
        plugins: { legend: { display: false } },
      },
    });
  });
}
document.addEventListener("DOMContentLoaded", initDonutCharts);

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
        initDashboardCharts();
        initDonutCharts();
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

  // Calendar dropdown for form.DateField/DateTime, on the root element
  // carrying data-value (the field's current "YYYY-MM-DD" or
  // "YYYY-MM-DDTHH:MM" string). Whether it also handles time is inferred
  // from the presence of the x-ref="time" input, so there is nothing else
  // to configure from the server side.
  Alpine.data("datePicker", function () {
    return {
      value: "",
      time: "",
      withTime: false,
      open: false,
      viewYear: 0,
      viewMonth: 0,
      focused: null,
      weekdays: weekdayLabels(),
      init: function () {
        this.value = this.$el.dataset.value || "";
        this.withTime = !!this.$refs.time;
        if (this.withTime && this.value.length > 10) this.time = this.value.slice(11, 16);
        var d = this.value ? isoParts(this.value) : todayParts();
        this.viewYear = d.y;
        this.viewMonth = d.m;
      },
      toggle: function () {
        if (this.open) {
          this.close();
          return;
        }
        var d = this.value ? isoParts(this.value) : todayParts();
        this.viewYear = d.y;
        this.viewMonth = d.m;
        this.focused = null;
        this.open = true;
        this.focusTarget();
      },
      close: function () {
        if (!this.open) return;
        this.open = false;
        this.$refs.trigger.focus();
      },
      prevMonth: function () {
        this.shiftMonth(-1);
      },
      nextMonth: function () {
        this.shiftMonth(1);
      },
      shiftMonth: function (delta) {
        var m = this.viewMonth + delta,
          y = this.viewYear;
        if (m < 0) {
          m = 11;
          y -= 1;
        } else if (m > 11) {
          m = 0;
          y += 1;
        }
        this.viewMonth = m;
        this.viewYear = y;
      },
      monthLabel: function () {
        return new Intl.DateTimeFormat(undefined, { month: "long", year: "numeric" }).format(
          new Date(this.viewYear, this.viewMonth, 1)
        );
      },
      grid: function () {
        var first = new Date(this.viewYear, this.viewMonth, 1);
        var start = new Date(this.viewYear, this.viewMonth, 1 - first.getDay());
        var selected = this.value ? this.value.slice(0, 10) : null;
        var today = isoDate(new Date());
        var cells = [];
        for (var i = 0; i < 42; i++) {
          var d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
          var iso = isoDate(d);
          cells.push({
            iso: iso,
            day: d.getDate(),
            inMonth: d.getMonth() === this.viewMonth,
            isToday: iso === today,
            selected: iso === selected,
          });
        }
        return cells;
      },
      isTabbable: function (cell) {
        return cell.iso === (this.focused || (this.value ? this.value.slice(0, 10) : isoDate(new Date())));
      },
      select: function (cell) {
        this.value = this.combine(cell.iso, this.time);
        this.close();
      },
      commitTime: function () {
        var iso = this.value ? this.value.slice(0, 10) : isoDate(new Date());
        this.value = this.combine(iso, this.time);
      },
      combine: function (iso, time) {
        return this.withTime ? iso + "T" + (time || "00:00") : iso;
      },
      today: function () {
        var t = todayParts();
        this.viewYear = t.y;
        this.viewMonth = t.m;
        this.select({ iso: isoDate(new Date(t.y, t.m, t.d)) });
      },
      clear: function () {
        this.value = "";
        this.time = "";
        this.close();
      },
      label: function () {
        if (!this.value) return "";
        var p = isoParts(this.value);
        var text = new Intl.DateTimeFormat(undefined, { year: "numeric", month: "short", day: "numeric" }).format(
          new Date(p.y, p.m, p.d)
        );
        return this.withTime && this.time ? text + " " + this.time : text;
      },
      move: function (delta, iso) {
        var p = isoParts(iso);
        var d = new Date(p.y, p.m, p.d + delta);
        this.viewYear = d.getFullYear();
        this.viewMonth = d.getMonth();
        this.focused = isoDate(d);
        this.focusTarget();
      },
      focusTarget: function () {
        var iso = this.focused || (this.value ? this.value.slice(0, 10) : isoDate(new Date()));
        this.$nextTick(
          function () {
            var el = this.$refs.panel && this.$refs.panel.querySelector('[data-iso="' + iso + '"]');
            if (el) el.focus();
          }.bind(this)
        );
      },
    };
  });
});
