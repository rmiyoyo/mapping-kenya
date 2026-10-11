(function () {
  if (!window.d3) return;

  var css = getComputedStyle(document.documentElement);
  function token(name) {
    return css.getPropertyValue(name).trim();
  }

  var whole = d3.format(",");
  var metrics = {
    population: { label: "Population, 2019", format: whole },
    density: { label: "People per km²", format: function (v) { return whole(Math.round(v)); } },
    voters: { label: "Registered voters, 2022", format: whole },
    area: { label: "Area, km²", format: d3.format(",.1f") }
  };
  var short = { population: "Population", density: "Density", voters: "Voters", area: "Area" };

  var tip = d3.select("body").append("div").attr("class", "tip").attr("aria-hidden", "true");

  function value(el, metric) {
    var v = el.getAttribute("data-" + metric);
    return v === null ? NaN : +v;
  }

  function showTip(event, el, extra) {
    tip.selectAll("*").remove();
    tip.append("strong").text(el.getAttribute("data-name"));
    var group = el.getAttribute("data-group");
    if (group) tip.append("span").attr("class", "tip-group").text(group);
    var rows = tip.append("dl");
    Object.keys(metrics).forEach(function (m) {
      var v = value(el, m);
      if (isNaN(v)) return;
      rows.append("dt").text(metrics[m].label);
      rows.append("dd").text(metrics[m].format(v));
    });
    if (extra) tip.append("span").attr("class", "tip-group").text(extra);
    moveTip(event);
    tip.classed("on", true);
  }

  function moveTip(event) {
    var node = tip.node();
    var x = event.clientX + 14;
    var y = event.clientY + 14;
    if (x + node.offsetWidth > window.innerWidth - 8) x = event.clientX - node.offsetWidth - 14;
    if (y + node.offsetHeight > window.innerHeight - 8) y = event.clientY - node.offsetHeight - 14;
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
    shapes.selectAll("title").remove();

    var zoom = d3.zoom()
      .scaleExtent([1, 16])
      .translateExtent([[0, 0], [box.width, box.height]])
      .filter(function (event) {
        return (event.type !== "wheel" || event.ctrlKey || event.metaKey) && !event.button;
      })
      .on("zoom", function (event) {
        layer.attr("transform", event.transform);
        marker.attr("r", 5 / event.transform.k);
        figure.classList.toggle("zoomed", event.transform.k > 1.01);
      });
    svg.call(zoom).on("dblclick.zoom", null);

    var tools = d3.select(figure).append("div").attr("class", "map-tools");
    tools.append("button").attr("type", "button").attr("aria-label", "Zoom in").text("+")
      .on("click", function () { svg.transition().duration(300).call(zoom.scaleBy, 1.8); });
    tools.append("button").attr("type", "button").attr("aria-label", "Zoom out").text("−")
      .on("click", function () { svg.transition().duration(300).call(zoom.scaleBy, 1 / 1.8); });
    tools.append("button").attr("type", "button").attr("class", "reset").text("Reset")
      .on("click", function () { svg.transition().duration(400).call(zoom.transform, d3.zoomIdentity); });
    d3.select(figure).append("p").attr("class", "map-hint").text("Drag to pan. Use the buttons, pinch or Ctrl and scroll to zoom.");

    function zoomTo(nodes, maxScale) {
      var b = null;
      nodes.forEach(function (n) {
        var r = n.getBBox();
        b = b ? [Math.min(b[0], r.x), Math.min(b[1], r.y), Math.max(b[2], r.x + r.width), Math.max(b[3], r.y + r.height)]
              : [r.x, r.y, r.x + r.width, r.y + r.height];
      });
      if (!b) return;
      var w = b[2] - b[0], h = b[3] - b[1];
      var k = Math.max(1, Math.min(maxScale, 0.7 / Math.max(w / box.width, h / box.height)));
      var t = d3.zoomIdentity.translate(box.width / 2, box.height / 2).scale(k).translate(-(b[0] + b[2]) / 2, -(b[1] + b[3]) / 2);
      svg.transition().delay(150).duration(750).call(zoom.transform, t);
    }

    if (figure.hasAttribute("data-focus")) {
      zoomTo(layer.selectAll("path.on").nodes(), 6);
    }

    var hooks = { enter: [], leave: [] };
    shapes
      .on("pointerenter", function (event) {
        d3.select(this).classed("hover", true).raise();
        if (this.parentNode.tagName.toLowerCase() === "a") d3.select(this.parentNode).raise();
        showTip(event, this);
        var el = this;
        hooks.enter.forEach(function (f) { f(el); });
      })
      .on("pointermove", moveTip)
      .on("pointerleave", function () {
        d3.select(this).classed("hover", false);
        hideTip();
        var el = this;
        hooks.leave.forEach(function (f) { f(el); });
      });
    marker.raise();
    layer.select(".outline").raise();

    return { figure: figure, shapes: shapes, hooks: hooks, zoomTo: zoomTo };
  }

  function choropleth(map) {
    var list = map.figure.getAttribute("data-metrics");
    if (!list) return;
    var keys = list.split(" ").filter(function (m) { return metrics[m]; });
    if (!keys.length) return;

    var controls = d3.select(map.figure).insert("div", ":first-child").attr("class", "map-controls");
    var legend = d3.select(map.figure).append("div").attr("class", "legend");

    if (keys.length > 1) {
      var group = controls.append("div").attr("class", "switch").attr("role", "group").attr("aria-label", "Shade the map by");
      group.selectAll("button").data(keys).join("button")
        .attr("type", "button")
        .text(function (m) { return short[m]; })
        .on("click", function (event, m) { apply(m, true); });
    }

    var colours = d3.quantize(d3.interpolateLab(token("--scale-lo"), token("--scale-hi")), 6);

    function apply(metric, animate) {
      controls.selectAll("button").attr("aria-pressed", function (m) { return m === metric; });
      var values = map.shapes.nodes().map(function (n) { return value(n, metric); }).filter(function (v) { return !isNaN(v); });
      var scale = d3.scaleQuantile().domain(values).range(colours);
      var fill = function () {
        var v = value(this, metric);
        return isNaN(v) ? token("--rule") : scale(v);
      };
      if (animate) map.shapes.transition().duration(450).style("fill", fill);
      else map.shapes.style("fill", fill);
      drawLegend(metric, scale);
    }

    function drawLegend(metric, scale) {
      legend.selectAll("*").remove();
      legend.append("span").attr("class", "legend-title").text(metrics[metric].label);
      var w = 280, h = 10, cuts = scale.quantiles(), extent = d3.extent(scale.domain());
      var stops = [extent[0]].concat(cuts, [extent[1]]);
      var x = d3.scaleLinear().domain([0, colours.length]).range([0, w]);
      var svg = legend.append("svg").attr("viewBox", "0 0 " + (w + 20) + " 32").attr("aria-hidden", "true");
      var g = svg.append("g").attr("transform", "translate(10,2)");
      g.selectAll("rect").data(colours).join("rect")
        .attr("x", function (c, i) { return x(i); })
        .attr("width", w / colours.length)
        .attr("height", h)
        .attr("fill", function (c) { return c; });
      var tick = d3.format(metric === "density" || metric === "area" ? ",.2~s" : ".2~s");
      g.selectAll("text").data(stops).join("text")
        .attr("x", function (v, i) { return x(i); })
        .attr("y", h + 14)
        .attr("text-anchor", "middle")
        .text(function (v) { return tick(v); });
    }

    apply(keys[0], false);
  }

  function linkTable(map) {
    var rows = d3.selectAll("tr[data-key]");
    if (rows.empty()) return;
    var byKey = {};
    map.shapes.each(function () { byKey[this.getAttribute("data-key")] = this; });
    rows
      .on("mouseenter", function () {
        var el = byKey[this.getAttribute("data-key")];
        if (el) d3.select(el).classed("hover", true).raise();
      })
      .on("mouseleave", function () {
        var el = byKey[this.getAttribute("data-key")];
        if (el) d3.select(el).classed("hover", false);
      });
    map.hooks.enter.push(function (el) {
      rows.filter(function () { return this.getAttribute("data-key") === el.getAttribute("data-key"); }).classed("hl", true);
    });
    map.hooks.leave.push(function () { rows.classed("hl", false); });
  }

  function constituencyChart(figure, map) {
    var data = JSON.parse(figure.querySelector("script").textContent);
    data.sort(function (a, b) { return d3.descending(a.voters, b.voters); });

    var width = Math.max(280, Math.round(figure.getBoundingClientRect().width)) || 400;
    var bar = 18, gap = 5, label = Math.min(150, Math.round(width * 0.36)), right = 64;
    var height = data.length * (bar + gap) + 24;
    var x = d3.scaleLinear().domain([0, d3.max(data, function (d) { return d.voters; })]).nice().range([label, width - right]);

    var svg = d3.select(figure).insert("svg", "figcaption")
      .attr("viewBox", "0 0 " + width + " " + height)
      .attr("role", "img")
      .attr("aria-label", "Registered voters by constituency");

    svg.append("g").attr("class", "axis").attr("transform", "translate(0,18)")
      .call(d3.axisTop(x).ticks(5, "~s").tickSize(-(height - 18)))
      .call(function (g) { g.select(".domain").remove(); });

    var rows = svg.selectAll("a.bar").data(data).join("a")
      .attr("class", "bar")
      .attr("href", function (d) { return d.href; })
      .attr("transform", function (d, i) { return "translate(0," + (24 + i * (bar + gap)) + ")"; });

    rows.append("text").attr("class", "name").attr("x", label - 8).attr("y", bar / 2).attr("dy", "0.35em")
      .attr("text-anchor", "end").text(function (d) { return d.name; });
    rows.append("rect").attr("x", label).attr("height", bar).attr("width", 0)
      .transition().duration(700).delay(function (d, i) { return i * 40; })
      .attr("width", function (d) { return x(d.voters) - label; });
    rows.append("text").attr("class", "value").attr("y", bar / 2).attr("dy", "0.35em")
      .attr("x", function (d) { return x(d.voters) + 6; })
      .text(function (d) { return whole(d.voters); });

    if (!map) return;
    function mark(name, on) {
      map.shapes.classed("grp", function () { return on && this.getAttribute("data-group") === name; });
      rows.classed("hl", function (d) { return on && d.name === name; });
    }
    rows
      .on("mouseenter focus", function (event, d) { mark(d.name, true); })
      .on("mouseleave blur", function () { mark("", false); });
    map.hooks.enter.push(function (el) { mark(el.getAttribute("data-group"), true); });
    map.hooks.leave.push(function () { mark("", false); });
  }

  var maps = [];
  d3.selectAll("figure.map").each(function () {
    var m = enhanceMap(this);
    if (!m) return;
    maps.push(m);
    choropleth(m);
  });
  if (maps.length) linkTable(maps[0]);
  d3.selectAll("figure.chart[data-chart=constituencies]").each(function () {
    constituencyChart(this, maps[0]);
  });
})();
