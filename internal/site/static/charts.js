(function () {
  if (!window.d3) return;

  var whole = d3.format(",");
  var metrics = {
    population: { label: "Population, 2019", heading: "population", format: whole },
    density: { label: "People per km²", heading: "density", format: function (v) { return whole(Math.round(v)); } },
    voters: { label: "Registered voters, 2022", heading: "registered voters", format: whole },
    area: { label: "Area, km²", heading: "area", format: d3.format(",.1f") }
  };
  var short = { population: "Population", density: "Density", voters: "Voters", area: "Area" };
  var steps = 6;

  function shade(i) {
    var pct = Math.round((i / (steps - 1)) * 100);
    return "color-mix(in oklab, var(--scale-lo), var(--scale-hi) " + pct + "%)";
  }

  var tip = d3.select("body").append("div").attr("class", "tip").attr("aria-hidden", "true");

  function value(el, metric) {
    var v = el.getAttribute("data-" + metric);
    return v === null || v === "" ? NaN : +v;
  }

  function showTip(event, el) {
    tip.selectAll("*").remove();
    tip.append("strong").text(el.getAttribute("data-name"));
    var group = el.getAttribute("data-group");
    if (group) tip.append("span").attr("class", "tip-group").text(group + " constituency");
    var rows = tip.append("dl");
    Object.keys(metrics).forEach(function (m) {
      var v = value(el, m);
      if (isNaN(v)) return;
      rows.append("dt").text(metrics[m].label);
      rows.append("dd").text(metrics[m].format(v));
    });
    if (rows.selectAll("dt").empty()) rows.remove();
    moveTip(event);
    tip.classed("on", true);
  }

  function moveTip(event) {
    var node = tip.node();
    var x = event.clientX + 16;
    var y = event.clientY + 16;
    if (x + node.offsetWidth > window.innerWidth - 8) x = event.clientX - node.offsetWidth - 16;
    if (y + node.offsetHeight > window.innerHeight - 8) y = event.clientY - node.offsetHeight - 16;
    tip.style("left", x + "px").style("top", y + "px");
  }

  function hideTip() {
    tip.classed("on", false);
  }

  function enhanceMap(figure) {
    var svg = d3.select(figure).select("svg");
    var layer = svg.select("g.layer");
    if (layer.empty()) return null;
    var box = svg.node().viewBox.baseVal;
    var shapes = layer.selectAll("path[data-key]");
    var marker = layer.select(".marker");
    var labels = layer.selectAll(".water-label");
    shapes.selectAll("title").remove();

    var zoom = d3.zoom()
      .scaleExtent([1, 16])
      .translateExtent([[0, 0], [box.width, box.height]])
      .filter(function (event) {
        return (event.type !== "wheel" || event.ctrlKey || event.metaKey) && !event.button;
      })
      .on("zoom", function (event) {
        var k = event.transform.k;
        layer.attr("transform", event.transform);
        marker.attr("r", 5 / k);
        labels.style("font-size", 11 / k + "px");
        figure.classList.toggle("zoomed", k > 1.01);
      });
    svg.call(zoom).on("dblclick.zoom", null);

    var tools = d3.select(figure).append("div").attr("class", "map-tools")
      .attr("title", "Drag to pan. Pinch, or hold Ctrl and scroll, to zoom.");
    tools.append("button").attr("type", "button").attr("aria-label", "Zoom in").text("+")
      .on("click", function () { svg.transition().duration(300).call(zoom.scaleBy, 1.8); });
    tools.append("button").attr("type", "button").attr("aria-label", "Zoom out").text("−")
      .on("click", function () { svg.transition().duration(300).call(zoom.scaleBy, 1 / 1.8); });
    tools.append("button").attr("type", "button").attr("class", "reset").attr("aria-label", "Reset zoom").text("↺")
      .on("click", function () { svg.transition().duration(400).call(zoom.transform, d3.zoomIdentity); });

    if (figure.hasAttribute("data-focus")) {
      var b = null;
      layer.selectAll("path.on").each(function () {
        var r = this.getBBox();
        b = b ? [Math.min(b[0], r.x), Math.min(b[1], r.y), Math.max(b[2], r.x + r.width), Math.max(b[3], r.y + r.height)]
              : [r.x, r.y, r.x + r.width, r.y + r.height];
      });
      if (b) {
        var k = Math.max(1, Math.min(6, 0.7 / Math.max((b[2] - b[0]) / box.width, (b[3] - b[1]) / box.height)));
        svg.call(zoom.transform, d3.zoomIdentity.translate(box.width / 2, box.height / 2).scale(k).translate(-(b[0] + b[2]) / 2, -(b[1] + b[3]) / 2));
      }
    }

    var hooks = { enter: [], leave: [] };
    shapes
      .on("pointerenter", function (event) {
        var el = this;
        var host = el.parentNode.tagName.toLowerCase() === "a" ? el.parentNode : el;
        d3.select(el).classed("hover", true);
        d3.select(host).raise();
        layer.selectAll(".water, .water-label, .outline, .marker").raise();
        showTip(event, el);
        hooks.enter.forEach(function (f) { f(el); });
      })
      .on("pointermove", moveTip)
      .on("pointerleave", function () {
        var el = this;
        d3.select(el).classed("hover", false);
        hideTip();
        hooks.leave.forEach(function (f) { f(el); });
      });

    return { figure: figure, shapes: shapes, hooks: hooks, listeners: [] };
  }

  function choropleth(map) {
    var list = map.figure.getAttribute("data-metrics");
    if (!list) return;
    var keys = list.split(" ").filter(function (m) { return metrics[m]; });
    if (!keys.length) return;

    var slot = document.querySelector("[data-map-controls]");
    var controls = slot ? d3.select(slot) : d3.select(map.figure).insert("div", ":first-child");
    var legend = d3.select(map.figure).append("div").attr("class", "legend");

    if (keys.length > 1) {
      controls.append("div").attr("class", "switch").attr("role", "group").attr("aria-label", "Shade the map by")
        .selectAll("button").data(keys).join("button")
        .attr("type", "button")
        .text(function (m) { return short[m]; })
        .on("click", function (event, m) { apply(m); });
    }

    function apply(metric) {
      controls.selectAll("button").attr("aria-pressed", function (m) { return m === metric; });
      var values = map.shapes.nodes().map(function (n) { return value(n, metric); }).filter(function (v) { return !isNaN(v); });
      var scale = d3.scaleQuantile().domain(values).range(d3.range(steps));
      map.shapes.style("fill", function () {
        var v = value(this, metric);
        return isNaN(v) ? "var(--paper-2)" : shade(scale(v));
      });
      drawLegend(metric, scale);
      map.listeners.forEach(function (f) { f(metric); });
    }

    function drawLegend(metric, scale) {
      legend.selectAll("*").remove();
      legend.append("span").attr("class", "legend-title").text(metrics[metric].label);
      var w = 260, h = 8;
      var extent = d3.extent(scale.domain());
      var stops = [extent[0]].concat(scale.quantiles(), [extent[1]]);
      var x = d3.scaleLinear().domain([0, steps]).range([0, w]);
      var svg = legend.append("svg").attr("viewBox", "0 0 " + (w + 24) + " 28").attr("aria-hidden", "true");
      var g = svg.append("g").attr("transform", "translate(12,2)");
      g.selectAll("rect").data(d3.range(steps)).join("rect")
        .attr("x", function (i) { return x(i); })
        .attr("width", w / steps - 1)
        .attr("height", h)
        .attr("rx", 1)
        .style("fill", shade);
      var tick = d3.format(metric === "density" ? ",.0f" : ".2~s");
      g.selectAll("text").data(stops).join("text")
        .attr("x", function (v, i) { return x(i); })
        .attr("y", h + 13)
        .attr("text-anchor", "middle")
        .text(function (v) { return tick(v); });
    }

    map.apply = apply;
    map.firstMetric = keys[0];
  }

  function ranking(map) {
    var list = document.querySelector("ol.ranking:not(.constituencies)");
    if (!list) return;
    var label = d3.select("[data-metric-label]");
    map.listeners.push(function (metric) {
      label.text(metrics[metric].heading);
      var items = Array.prototype.slice.call(list.querySelectorAll("li[data-key]"));
      var top = d3.max(items, function (n) { return value(n, metric); }) || 1;
      items.sort(function (a, b) { return d3.descending(value(a, metric), value(b, metric)); });
      items.forEach(function (n, i) {
        var v = value(n, metric);
        list.appendChild(n);
        n.hidden = i >= 10;
        n.style.setProperty("--share", (v / top).toFixed(4));
        n.querySelector(".value").textContent = metrics[metric].format(v);
      });
    });
  }

  function link(map) {
    var byKey = {};
    map.shapes.each(function () { byKey[this.getAttribute("data-key")] = this; });
    var rows = d3.selectAll("tr[data-key], li[data-key]");
    rows
      .on("mouseenter", function () {
        var el = byKey[this.getAttribute("data-key")];
        if (el) d3.select(el).classed("hover", true);
      })
      .on("mouseleave", function () {
        var el = byKey[this.getAttribute("data-key")];
        if (el) d3.select(el).classed("hover", false);
      });

    var groups = d3.selectAll("li[data-group]");
    function mark(name) {
      map.shapes.classed("grp", function () { return name !== null && this.getAttribute("data-group") === name; });
      groups.classed("hl", function () { return name !== null && this.getAttribute("data-group") === name; });
    }
    groups
      .on("mouseenter", function () { mark(this.getAttribute("data-group")); })
      .on("mouseleave", function () { mark(null); });

    map.hooks.enter.push(function (el) {
      var key = el.getAttribute("data-key");
      var group = el.getAttribute("data-group");
      rows.classed("hl", function () { return this.getAttribute("data-key") === key; });
      groups.classed("hl", function () { return group !== null && this.getAttribute("data-group") === group; });
    });
    map.hooks.leave.push(function () {
      rows.classed("hl", false);
      groups.classed("hl", false);
    });
  }

  var first = null;
  d3.selectAll("figure.map").each(function () {
    var m = enhanceMap(this);
    if (!m) return;
    choropleth(m);
    if (!first) first = m;
  });
  if (first) {
    ranking(first);
    link(first);
    if (first.apply) first.apply(first.firstMetric);
  }
})();
